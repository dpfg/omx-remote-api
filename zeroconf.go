package main

import (
	"log"

	"github.com/grandcat/zeroconf"
)

const (
	zeroConfName    = "OMX Remote"
	zeroConfService = "_omx-remote-api._tcp"
	zeroConfDomain  = "local."
)

func startZeroConfService(port int, version string) (*zeroconf.Server, error) {
	log.Printf("Starting zeroconf service [%s]\n", zeroConfName)
	return zeroconf.Register(zeroConfName, zeroConfService, zeroConfDomain, port, []string{"version=" + version}, nil)
}
