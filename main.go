package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	log "github.com/Sirupsen/logrus"
)

const ZN_PATH string = "ZeroNet"
const ZN_DATA string = "data"
const ZN_DATA_ALT string = "data_alt"
const ZN_HOMEPAGE string = "1HeLLo4uzjaLetFx6NH3PMwFP3qbRbTf3D"

var PEER_ID string
var DATA string

// ZeroNet version mimicry
const VERSION string = "0.5.1"
const REV int = 1756

func main() {
	DATA = path.Join(".", "data")
	os.MkdirAll(DATA, 0777)
	createCerts()
	PEER_ID = fmt.Sprintf("-ZN0%s-GO%s", strings.Replace(VERSION, ".", "", -1), randomString(10))
	log.WithFields(log.Fields{
		"id": PEER_ID,
	}).Info("Your Peer ID")
	sm := NewSiteManager()
	server := NewServer(43111, sm)
	server.Serve()
}
