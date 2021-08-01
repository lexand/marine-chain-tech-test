package main

import (
	"flag"
	log "go.uber.org/zap"
	"os"
	"os/signal"
	"storage/common"
	"storage/private"
	"storage/public"
	"syscall"
)

const (
	modePublic  = "public"
	modePrivate = "private"
)

var (
	mode       = flag.String("mode", "", "Mode for instance 'public' - receive/send files, 'private' - receive/send save file parts")
	path       = flag.String("path", GetDefaultPath(), "For 'private' instance. Path to directory where files will be stored. If you want to run several instances on the same host please specify separate directory for each instance")
	listen     = flag.String("listen", "127.0.0.1:8888", "Specify network address to listen for")
	publicAddr = flag.String("public", "http://127.0.0.1:8888", "For 'private' instance. Address to 'public instance'")
	id         = flag.String("id", "0000", "For 'private' instance.Instance ID, should be unique for all private instances")
)

var (
	instance common.Instance
	logger   *log.Logger

	awaitChan  = make(chan struct{}, 1)
	signalChan = make(chan os.Signal, 1)
)

func main() {
	flag.Parse()

	logger = log.NewExample()

	checkFlags()

	initInstance()

	listenSignals(shutdown)

	startInstance()

	AwaitStop()
}

func startInstance() {
	err := instance.Start()
	if err != nil {
		logger.Fatal("cant start instance", log.Error(err), log.String("mode", *mode))
	}
}

func initInstance() {
	switch *mode {
	case modePublic:
		instance = public.NewInstance(*listen, logger.Named("PUB"))
	case modePrivate:
		var err error
		instance, err = private.NewInstance(*id, *listen, *path, *publicAddr, logger.Named("PRV"))
		if err != nil {
			logger.Fatal("cant run private instance", log.Error(err))
		}
	default:
		logger.Fatal("unknown mode", log.String("mode", *mode))
	}
}

func listenSignals(shutdownFunc func()) {
	signal.Notify(signalChan, syscall.SIGTERM, os.Interrupt)

	go func() {
		<-signalChan
		logger.Debug("got termination signal, try to stop instance")
		shutdownFunc()
	}()
}

func checkFlags() {

	if *mode == modePrivate {
	}
}

func shutdown() {
	instance.Stop()
	awaitChan <- struct{}{}
	logger.Debug("instance stopped", log.String("mode", *mode))
}

func AwaitStop() {
	<-awaitChan
}

func GetDefaultPath() (path string) {
	path, _ = os.Getwd()
	return
}
