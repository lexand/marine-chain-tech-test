package private

import (
	"bytes"
	"context"
	"encoding/json"
	log "go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"os"
	"storage/common"
	"time"
)

type Private struct {
	publicAddr string
	path       string
	id         string
	listenAddr string
	srv        *http.Server
	cl         *http.Client
	logger     *log.Logger
}

var _ common.Instance = (*Private)(nil)

func NewInstance(id, listenAddr, path, publicAddr string, logger *log.Logger) (*Private, error) {

	if len(id) != 4 {
		return nil, common.ErrBadPrivateId
	}

	fs, err := os.Stat(path)
	if err != nil {
		logger.Error("IO error", log.Error(err))
		return nil, common.ErrBadPath
	}
	if !fs.IsDir() {
		logger.Error("'path' argument must point to directory not a file", log.String("path", path))
		return nil, common.ErrBadPath
	}
	if fs.Mode().Perm()&(1<<(uint(7))) == 0 {
		logger.Error("'path' should be writable", log.String("path", path))
		return nil, common.ErrBadPath
	}

	if publicAddr == "" {
		logger.Warn("public address empty")
	}

	inst := &Private{
		id:         id,
		publicAddr: publicAddr,
		listenAddr: listenAddr,
		path:       path,
		cl: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				MaxConnsPerHost:       1,
				IdleConnTimeout:       0,
				ResponseHeaderTimeout: 5 * time.Second,
				ForceAttemptHTTP2:     false,
			},
		},
		logger: logger,
	}
	inst.srv = &http.Server{
		Addr:    listenAddr,
		Handler: http.HandlerFunc(inst.handler),
	}
	return inst, nil
}

func (p *Private) Start() (err error) {
	go func() {
		// info: or we can use custom TCP listener with TCP_FASTOPEN, TCP_DEFER_ACCEPT and with redefined backlog size
		err = p.srv.ListenAndServe()
	}()

	time.Sleep(10 * time.Millisecond) // warn: use explicit wait is bad idea but works well in most cases

	err = p.registerInPublic()
	if err != nil {
		_ = p.srv.Shutdown(context.Background())
		return err
	}

	return
}

func (p *Private) Stop() {
	_ = p.srv.Shutdown(context.Background())
}

func (p *Private) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "PUT" {
		fileName := r.URL.Query().Get("filename")
		// todo: need to check checksum before saving, this does only on the private side to check network

		err := p.saveFile(fileName, r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			p.logger.Error("save file", log.Error(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method == "GET" {
		fileName := r.URL.Query().Get("filename")

		rd, err := p.loadFile(fileName)
		if err == os.ErrNotExist {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			p.logger.Error("load file", log.Error(err))
			return
		}

		defer rd.Close()

		// todo: need to check checksum before sending, this is better to do on private side to check storage consistency
		//   and on the public side to check network
		// warn: this cause to "http: superfluous response.WriteHeader call from)" at line 141
		_, err = io.Copy(w, rd)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			p.logger.Error("send file to public", log.Error(err))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("content-type", "application/octet-stream")

		return
	}

	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte("method not supported"))
}

func (p *Private) saveFile(fileName string, body io.ReadCloser) (err error) {
	err = p.checkFileName(fileName)
	if err != nil {
		return err
	}

	var f *os.File
	// currently, we will store all file flat - only in one directory
	fullFilePath := p.path + "/" + fileName
	f, err = os.Create(fullFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, body)
	if err != nil {
		return err
	}
	return nil
}

func (p *Private) loadFile(fileName string) (f io.ReadCloser, err error) {
	err = p.checkFileName(fileName)
	if err != nil {
		return nil, err
	}

	fullFilePath := p.path + "/" + fileName
	f, err = os.Open(fullFilePath)
	if err != nil {
		return nil, err
	}
	return
}

func (p *Private) checkFileName(fileName string) error {
	if fileName == "" {
		return common.ErrFileNameEmpty
	}

	// todo: perform additional check for file name struct

	return nil
}

func (p *Private) registerInPublic() error {
	if p.publicAddr == "" {
		return nil
	}

	addr, _ := net.ResolveTCPAddr("tcp4", p.listenAddr)

	pi := common.PrivateInfo{
		ID:   p.id,
		Port: uint32(addr.Port),
	}
	buf := &bytes.Buffer{}
	je := json.NewEncoder(buf)
	_ = je.Encode(&pi)

	_, err := p.cl.Post(
		p.publicAddr+"/register",
		"application/json",
		buf,
	)
	if err != nil {
		p.logger.Error("cant register private instance from public", log.Error(err))
		return common.ErrCantRegister
	}
	return nil
}
