package main

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	log "go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"storage/private"
	"storage/public"
	"testing"
	"time"
)

func Test_PUTGET(t *testing.T) {
	logger := log.NewExample()

	wd, _ := os.Getwd()
	_ = os.Remove(wd + "/test.00")
	_ = os.Remove(wd + "/test.01")
	_ = os.Remove(wd + "/test.02")
	_ = os.Remove(wd + "/test.03")
	_ = os.Remove(wd + "/test.04")

	pub := public.NewInstance("127.0.0.1:8000", logger.Named("PUB"))
	_ = pub.Start()
	time.Sleep(1 * time.Millisecond)

	for i := 0; i < 5; i++ {
		prv, err := private.NewInstance(
			fmt.Sprintf("%04d", i),
			"127.0.0.1:8100",
			wd,
			"http://127.0.0.1:8000",
			logger.Named(fmt.Sprintf("PRV.%04d", i)))
		assert.NoError(t, err)
		prv.Start()
		time.Sleep(1 * time.Millisecond)
	}

	refFileData := []byte{1, 2, 3, 4, 5, 6, 7}

	cl := &http.Client{}

	rq, _ := http.NewRequest("PUT", "http://127.0.0.1:8000?filename=test", bytes.NewBuffer(refFileData))
	resp, err := cl.Do(rq)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	time.Sleep(1 * time.Millisecond)

	resp, err = cl.Get("http://127.0.0.1:8000?filename=test")
	assert.NoError(t, err)
	defer resp.Body.Close()
	resFileData, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, refFileData, resFileData)
}
