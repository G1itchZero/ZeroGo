package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/G1itchZero/zeronet-go/server"
	"github.com/G1itchZero/zeronet-go/site_manager"
	"github.com/G1itchZero/zeronet-go/utils"
	log "github.com/Sirupsen/logrus"
	"github.com/pkg/browser"
)

func main() {
	os.MkdirAll(utils.GetDataPath(), 0777)
	utils.CreateCerts()
	log.SetLevel(log.WarnLevel)
	log.WithFields(log.Fields{
		"id": utils.GetPeerID(),
	}).Info("Your Peer ID")

	debug := flag.Bool("debug", false, "debug mode")
	port := flag.Int("port", 43111, "serving port")
	flag.Parse()
	sm := site_manager.NewSiteManager()
	hasMedia, _ := utils.Exists(path.Join(utils.GetDataPath(), utils.ZN_UPDATE))

	if *debug {
		log.SetLevel(log.DebugLevel)
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if !hasMedia {
		site := sm.GetFiles(utils.ZN_UPDATE, func(filename string) bool {
			return strings.HasPrefix(filename, utils.ZN_MEDIA)
		})
		site.Wait()
	}

	s := server.NewServer(*port, sm)
	go func() {
		time.Sleep(time.Second)
		browser.OpenURL(fmt.Sprintf("http://127.0.0.1:%d", *port))
	}()
	s.Serve()
}
