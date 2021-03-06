package site_manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/G1itchZero/ZeroGo/downloader"
	"github.com/G1itchZero/ZeroGo/site"
	"github.com/G1itchZero/ZeroGo/utils"
	"github.com/Jeffail/gabs"
	log "github.com/Sirupsen/logrus"
	"gopkg.in/cheggaaa/pb.v1"
)

type SiteManager struct {
	Sites  map[string]*site.Site
	Names  map[string]interface{}
	pbPool *pb.Pool
}

func NewSiteManager() *SiteManager {
	pool, _ := pb.StartPool()
	sm := SiteManager{
		Sites:  map[string]*site.Site{},
		pbPool: pool,
	}
	go sm.updateSites()
	return &sm
}

func (sm *SiteManager) LoadNames() {
	log.Info("Loading .bit names...")
	names, err := utils.LoadJSON(path.Join(utils.GetDataPath(), utils.ZN_NAMES, "data/names.json"))
	if err != nil {
		log.Fatal(fmt.Errorf("Error name resolving: %v", err))
	}
	sm.Names = names.Data().(map[string]interface{})
}

func (sm *SiteManager) Remove(address string) {
	site := sm.Sites[address]
	go site.Remove()
	delete(sm.Sites, address)
	sm.SaveSites()
}

func (sm *SiteManager) Get(address string) *site.Site {
	s, ok := sm.Sites[address]
	if !ok {
		var bit string
		if strings.HasSuffix(address, ".bit") {
			bit = address
			address, ok = sm.Names[address].(string)
			if !ok {
				return nil
			}
		}
		s = site.NewSite(address)
		s.Added = int(time.Now().Unix())
		sm.Sites[address] = s
		if bit != "" {
			sm.Sites[bit] = s
		}
	}
	if !utils.GetDebug() {
		sm.pbPool.Add(s.Downloader.ProgressBar)
	}
	go sm.processSite(s)
	return s
}

func (sm *SiteManager) GetFiles(address string, filter downloader.FilterFunc) *site.Site {
	s, ok := sm.Sites[address]
	if !ok {
		s = site.NewSite(address)
		s.Filter = filter
		s.Added = int(time.Now().Unix())
		sm.Sites[address] = s
	}
	if !utils.GetDebug() {
		sm.pbPool.Add(s.Downloader.ProgressBar)
	}
	go sm.processSite(s)
	return s
}

func (sm *SiteManager) processSite(s *site.Site) {
	done := make(chan *site.Site, 2)
	s.Download(done)
	s.Wait()
	if s.Filter == nil {
		sm.SaveSites()
	}
}

func (sm *SiteManager) SaveSites() {
	sites := sm.GetSites()
	// log.Fatal(sites)
	filename := path.Join(utils.GetDataPath(), "sites.json")
	ioutil.WriteFile(filename, []byte(sites.StringIndent("", "  ")), 0644)
}

func (sm *SiteManager) GetSites() *gabs.Container {
	sites := gabs.New()
	for addr, s := range sm.Sites {
		if s.Content != nil && s.Filter == nil && !strings.HasSuffix(addr, ".bit") {
			sites.Set(s.GetInfo(), addr)
		}
	}
	return sites
}

func (sm *SiteManager) updateSites() {
	s, _ := loadSites()
	if s != nil {
		sites, _ := s.ChildrenMap()
		for address, content := range sites {
			log.WithFields(log.Fields{
				"address": address,
			}).Debug("Preload site")
			sm.Sites[address] = site.NewSite(address)
			sm.Sites[address].LastPeers = int(content.S("peers").Data().(float64))
			sm.Sites[address].LastContent = content.S("content")
		}
	}
	log.Info("Sites preloaded...")
}

func loadSites() (*gabs.Container, error) {
	filename := path.Join(utils.GetDataPath(), "sites.json")
	if _, err := os.Stat(filename); err != nil {
		jsonObj := gabs.New()
		ioutil.WriteFile(filename, []byte(jsonObj.String()), 0644)
	}
	return utils.LoadJSON(filename)
}
