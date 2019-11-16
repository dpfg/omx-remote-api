package main

import (
	"io"

	"github.com/sirupsen/logrus"
)

// OmxProcessStatus provides player status by parsing process output
type OmxProcessStatus struct {
	Stdout io.Reader
	Stderr io.Reader

	Logger *logrus.Logger
}

// Start listening for std out and err and gather structured data
func (s *OmxProcessStatus) Start() {
	s.Logger.Debug("Start listening omx process status")
	// go func() { debugger(s.Stderr, s.Logger) }()
	go func() { debugger(s.Stdout, s.Logger.WithField("status", "stdout")) }()
	go func() { debugger(s.Stderr, s.Logger.WithField("status", "stderr")) }()
}

func debugger(pipe io.Reader, logger *logrus.Entry) {
	// buff := bufio.NewReader(pipe)

	// for {
	// 	data, err := buff.ReadBytes('\r')
	// 	if err != nil {
	// 		// logger.Debug(err.Error())
	// 		break
	// 	}
	// 	logger.Debug(string(data))
	// }
}
