package main

import (
	"os"
	"path"

	"net/http"
	_ "net/http/pprof"

	"github.com/G1itchZero/zeronet-go/server"
	"github.com/G1itchZero/zeronet-go/site_manager"
	"github.com/G1itchZero/zeronet-go/utils"
	log "github.com/Sirupsen/logrus"
)

func main() {
	os.MkdirAll(utils.GetDataPath(), 0777)
	utils.CreateCerts()
	log.SetLevel(log.InfoLevel)
	log.SetLevel(log.DebugLevel)
	log.WithFields(log.Fields{
		"id": utils.GetPeerID(),
	}).Info("Your Peer ID")
	sm := site_manager.NewSiteManager()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	hasMedia, _ := utils.Exists(path.Join(utils.GetDataPath(), utils.ZN_UPDATE))
	if !hasMedia {
		site := sm.Get(utils.ZN_UPDATE)
		site.Wait()
	}

	s := server.NewServer(43111, sm)
	s.Serve()
}
