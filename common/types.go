package common

type Instance interface {
	// Start
	//  should not block execution
	Start() error
	Stop()
}
