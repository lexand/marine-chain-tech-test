package public

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	log "go.uber.org/zap"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"storage/common"
	"sync"
)

type PrivateID string

type Private struct {
	ID   PrivateID
	Addr net.TCPAddr

	// todo: size written
	// todo: last heartbeat time
}

type Public struct {
	listenAddr string
	srv        *http.Server
	cl         *http.Client
	logger     *log.Logger

	privateMx    sync.RWMutex
	currentIdx   int
	privateQueue []PrivateID // the simplest solution, size will be eventually equally distributed
	privateSet   map[PrivateID]Private

	// this should be replaced by some permanent storage
	fileDbMx sync.RWMutex
	fileDb   map[string][]PrivateID
}

var _ common.Instance = (*Public)(nil)

func NewInstance(listenAddr string, logger *log.Logger) *Public {
	inst := &Public{
		listenAddr: listenAddr,
		logger:     logger,
		privateSet: make(map[PrivateID]Private, 5),
		fileDb:     make(map[string][]PrivateID),
		cl: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns: 5,
			},
		},
	}
	inst.srv = &http.Server{
		Addr:    inst.listenAddr,
		Handler: http.HandlerFunc(inst.handler),
	}
	return inst
}

func (p *Public) Start() (err error) {
	go func() {
		err = p.srv.ListenAndServe()
	}()
	return
}

func (p *Public) Stop() {
	_ = p.srv.Shutdown(context.Background())
}

func (p *Public) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "PUT" {
		p.saveFile(w, r)
		return
	}

	if r.Method == "GET" {
		p.loadFile(w, r)
		return
	}

	if r.Method == "POST" {
		if r.URL.Path == "/register" {
			p.register(w, r)
			return
		}
	}

	_, _ = w.Write([]byte("method not supported"))
	w.WriteHeader(http.StatusBadRequest)
}

func (p *Public) register(w http.ResponseWriter, r *http.Request) {
	je := json.NewDecoder(r.Body)

	pi := common.PrivateInfo{}

	err := je.Decode(&pi)
	if err != nil {
		p.logger.Error("cant parse register request", log.Error(err))
		_, _ = w.Write([]byte("error parsing register request:"))
		_, _ = w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	addr, err := net.ResolveTCPAddr("tcp4", r.RemoteAddr)
	if err != nil {
		p.logger.Error("cant parse remote addr", log.Error(err))
		_, _ = w.Write([]byte("error parsing remote addr:"))
		_, _ = w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	p.privateMx.RLock()
	if _, ok := p.privateSet[PrivateID(pi.ID)]; ok {
		p.logger.Warn("private instance already registered, but I have get register request", log.String("id", pi.ID))
		w.WriteHeader(http.StatusNoContent)
		p.privateMx.RUnlock()
		return
	}
	p.privateMx.RUnlock()

	p.privateMx.Lock()
	defer p.privateMx.Unlock()

	if _, ok := p.privateSet[PrivateID(pi.ID)]; !ok {
		p.privateQueue = append(p.privateQueue, PrivateID(pi.ID))
		addr.Port = int(pi.Port)
		p.privateSet[PrivateID(pi.ID)] = Private{
			ID:   PrivateID(pi.ID),
			Addr: *addr,
		}
		addr.Port = int(pi.Port)
		p.logger.Debug("private instance registered", log.String("addr", addr.String()), log.String("id", pi.ID))
	}

	w.WriteHeader(http.StatusNoContent)
}

func (p *Public) saveFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("filename")
	err := checkFileName(fileName)
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	p.fileDbMx.RLock()
	_, ok := p.fileDb[fileName]
	if ok {
		_, _ = w.Write([]byte("file already exists and cannot be rewritten"))
		w.WriteHeader(http.StatusBadRequest)
		p.fileDbMx.RUnlock()
		return
	}
	p.fileDbMx.RUnlock()

	p.fileDbMx.Lock()
	defer p.fileDbMx.Unlock()

	_, ok = p.fileDb[fileName]
	if ok {
		_, _ = w.Write([]byte("file already exists and cannot be rewritten"))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var privs []PrivateID
	p.privateMx.Lock()

	if len(p.privateQueue) < 5 {
		_, _ = w.Write([]byte("no quorum, service not ready"))
		w.WriteHeader(http.StatusServiceUnavailable)
		p.privateMx.Unlock()
		return
	}

	idx := p.currentIdx
	// p.currentIdx += 5
	p.currentIdx += 1 + rand.Intn(len(p.privateQueue)>>1)
	p.currentIdx %= len(p.privateQueue)

	for i := 0; i < 5; i++ {
		privs = append(privs, p.privateQueue[idx])
		idx += 1
		if idx >= len(p.privateQueue) {
			idx = 0
		}
	}

	p.privateMx.Unlock()

	p.fileDb[fileName] = privs

	// todo: not optimal, need to use stream-splitter reader-writer
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		_, _ = w.Write([]byte("cant read file from request"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(buf) < 5 {
		err = p.saveChunk(privs[0], buf, fileName, 0)
	} else {
		errs := make([]error, 5)
		chunkSize := len(buf) / 5
		var chunk []byte
		wg := sync.WaitGroup{}
		for i := 0; i < 4; i++ {
			chunk = buf[0:chunkSize]
			buf = buf[chunkSize:]
			wg.Add(1)
			go func(i int, chunk []byte) {
				err := p.saveChunk(privs[i], chunk, fileName, i)
				errs[i] = err
				if err != nil {
					addr := p.privateSet[privs[i]].Addr
					p.logger.Error("save chunk",
						log.Int("chunk_ind", i),
						log.Int("chunk_size", len(chunk)),
						log.String("private_instance", addr.String()),
					)
				}
				wg.Done()
			}(i, chunk)
		}
		errs[4] = p.saveChunk(privs[4], buf, fileName, 4)
		wg.Wait()

		failed := false
		for i, _ := range errs {
			if errs[i] != nil {
				failed = true
				break
			}
		}

		if failed {
			_, _ = w.Write([]byte("cant save file"))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (p *Public) loadFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("filename")
	err := checkFileName(fileName)
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	p.fileDbMx.RLock()
	defer p.fileDbMx.RUnlock()

	privs, ok := p.fileDb[fileName]
	if !ok {
		_, _ = w.Write([]byte("file not found"))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// todo:
	//  - use pools
	//  - if we store in storage file size, and splitting algorithm will be deterministic for saving and for loading
	//    we can use parallel chunks loading
	var buf []byte

	for i := range privs {
		t, err := p.loadChunk(privs[i], fileName, i)
		if err != nil {
			_, _ = w.Write([]byte("something wrong"))
			w.WriteHeader(http.StatusInternalServerError)
			break
		}
		buf = append(buf, t...)
	}

	_, _ = w.Write(buf)
	w.Header().Set("content-type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

func (p *Public) saveChunk(id PrivateID, chunk []byte, fileName string, chunkInd int) error {
	fileName = fmt.Sprintf("%s.%02d", fileName, chunkInd)
	addr := p.privateSet[id].Addr
	// todo: add urlencode for file names
	rq, err := http.NewRequest("PUT", fmt.Sprintf("http://%s/?filename=%s", addr.String(), fileName), bytes.NewBuffer(chunk))
	if err != nil {
		p.logger.Fatal("build request", log.Error(err))
	}
	resp, err := p.cl.Do(rq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return common.ErrExpectHttpNoContent
	}
	return nil
}
func (p *Public) loadChunk(id PrivateID, fileName string, chunkInd int) ([]byte, error) {
	fileName = fmt.Sprintf("%s.%02d", fileName, chunkInd)
	addr := p.privateSet[id].Addr
	// todo: add urlencode for file names
	resp, err := p.cl.Get(fmt.Sprintf("http://%s/?filename=%s", addr.String(), fileName))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, common.ErrExpectHttpOK
	}

	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func checkFileName(fileName string) error {
	if fileName == "" {
		return common.ErrFileNameEmpty
	}
	return nil
}
