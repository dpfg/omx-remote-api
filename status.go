package main

import (
	"bufio"
	"io"

	"github.com/sirupsen/logrus"
)

// OmxProcessStatus provides player status by parsing process output
type OmxProcessStatus struct {
	Stdout io.Reader
	Stderr io.Reader

	Logger *logrus.Logger
}

func (s *OmxProcessStatus) Start() {
	s.Logger.Debug("Start listening omx process status")
	// go func() { debugger(s.Stderr, s.Logger) }()
	go func() { debugger(s.Stdout, s.Logger) }()
}

func debugger(pipe io.Reader, logger *logrus.Logger) {
	buff := bufio.NewReader(pipe)

	for {
		data, err := buff.ReadBytes('\r')
		if err != nil {
			logger.Debug(err.Error())
			break
		}
		logger.Debug(string(data))
	}
}
