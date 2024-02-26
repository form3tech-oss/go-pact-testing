package pacttesting

import (
	"errors"

	"github.com/sirupsen/logrus"
)

var errFatal = errors.New("fatal error")

type FatalHandler struct {
	logger   *logrus.Logger
	exitCode int
}

func NewFatalHandler(logger *logrus.Logger) *FatalHandler {
	return &FatalHandler{
		logger: logger,
	}
}

func NewFatalHandlerDefault() *FatalHandler {
	return NewFatalHandler(logrus.StandardLogger())
}

func (h *FatalHandler) Handle(fn func()) {
	defer func(origExitFunc func(int)) {
		h.logger.ExitFunc = origExitFunc
		if err := recover(); err != nil {
			errTyped, ok := err.(error)
			if !ok || !errors.Is(errTyped, errFatal) {
				panic(err)
			}
		}
	}(h.logger.ExitFunc)

	h.exitCode = 0
	h.logger.ExitFunc = func(exitCode int) {
		h.exitCode = exitCode
		panic(errFatal)
	}
	fn()
}

func (h *FatalHandler) ExitCode() int {
	return h.exitCode
}
