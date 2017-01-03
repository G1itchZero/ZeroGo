package main

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/G1itchZero/ZeroGo/server"
	"github.com/G1itchZero/ZeroGo/site_manager"
	"github.com/G1itchZero/ZeroGo/utils"
	log "github.com/Sirupsen/logrus"
	"github.com/pkg/browser"
	"github.com/urfave/cli"
	l "log"
	"io/ioutil"
)

const VERSION string = "0.1.0"

func main() {
	os.MkdirAll(utils.GetDataPath(), 0777)
	utils.CreateCerts()
	log.SetLevel(log.ErrorLevel)
	l.SetOutput(ioutil.Discard)
	log.WithFields(log.Fields{
		"id": utils.GetPeerID(),
	}).Info("Your Peer ID")

	app := cli.NewApp()
  app.Name = "ZeroGo"
  app.Usage = "ZeroNet gate"
	app.Version = VERSION

	app.Flags = []cli.Flag {
    cli.BoolFlag{
      Name: "debug",
      Usage: "enable debug mode",
    },
    cli.BoolFlag{
      Name: "no-tab",
      Usage: "dont open new tab",
    },
    cli.IntFlag{
      Name: "port",
      Value: 43210,
      Usage: "serving port",
    },
    cli.StringFlag{
      Name: "homepage",
      Value: utils.ZN_HOMEPAGE,
      Usage: "homepage",
    },
  }

	sm := site_manager.NewSiteManager()
	hasMedia, _ := utils.Exists(path.Join(utils.GetDataPath(), utils.ZN_UPDATE))

	sync := make(chan int)
	if !hasMedia {
		go func() {
			site := sm.GetFiles(utils.ZN_UPDATE, func(filename string) bool {
				return strings.HasPrefix(filename, utils.ZN_MEDIA)
			})
			site.Wait()
			sync <- 0
		}()
	} else {
		go func() {
			sync <- 0
		}()
	}
	names := sm.GetFiles(utils.ZN_NAMES, func(filename string) bool {
		return strings.HasPrefix(filename, "data/names.json")
	})
	names.Wait()
	sm.Get(utils.ZN_ID)
	// ids.Wait()
	<-sync
	sm.LoadNames()

	app.Commands = []cli.Command{
    {
      Name:    "download",
      Aliases: []string{"d"},
      Usage:   "download site",
      Action:  func(c *cli.Context) error {
				if c.Bool("debug") {
					log.SetLevel(log.DebugLevel)
				}
				address := c.Args().First()
				sm.Remove(address)
				site := sm.Get(address)
				site.Wait()
        return nil
      },
    },
	}

  app.Action = func(c *cli.Context) error {
		utils.SetHomepage(c.String("homepage"))
		s := server.NewServer(c.Int("port"), sm)
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
			go func() {
				log.Println(http.ListenAndServe("localhost:6060", nil))
			}()
		}
		if !c.Bool("no-tab") {
			go func() {
				time.Sleep(time.Second)
				addr := utils.ZN_HOMEPAGE
				if c.NArg() > 0 {
					addr = c.Args().First()
				}
				browser.OpenURL(fmt.Sprintf("http://127.0.0.1:%d/%s", c.Int("port"), addr))
			}()
		}
		s.Serve()
    return nil
  }

  app.Run(os.Args)
}
