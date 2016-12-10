package site_manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/G1itchZero/zeronet-go/downloader"
	"github.com/G1itchZero/zeronet-go/site"
	"github.com/G1itchZero/zeronet-go/utils"
	"github.com/Jeffail/gabs"
	log "github.com/Sirupsen/logrus"
)

type SiteManager struct {
	Sites map[string]*site.Site
}

func NewSiteManager() *SiteManager {
	sm := SiteManager{
		Sites: map[string]*site.Site{},
	}
	go sm.updateSites()
	return &sm
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
		s = site.NewSite(address)
		s.Added = int(time.Now().Unix())
		sm.Sites[address] = s
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
	go sm.processSite(s)
	return s
}

func (sm *SiteManager) processSite(s *site.Site) {
	done := make(chan *site.Site, 2)
	s.Download(done)
	s.Wait()
	sm.SaveSites()
}

func (sm *SiteManager) SaveSites() {
	sites := sm.GetSites()
	// log.Fatal(sites)
	filename := path.Join(utils.GetDataPath(), "sites.json")
	fmt.Println(ioutil.WriteFile(filename, []byte(sites.StringIndent("", "  ")), 644))
}

func (sm *SiteManager) GetSites() *gabs.Container {
	sites := gabs.New()
	for addr, s := range sm.Sites {
		if s.Content != nil {
			sites.Set(s.GetInfo(), addr)
		}
	}
	return sites
}

func (sm *SiteManager) updateSites() {
	s, _ := loadSites()
	if s != nil {
		sites, _ := s.ChildrenMap()
		for address := range sites {
			log.WithFields(log.Fields{
				"address": address,
			}).Debug("Preload site")
			sm.Sites[address] = site.NewSite(address)
		}
	}
}

func loadSites() (*gabs.Container, error) {
	filename := path.Join(utils.GetDataPath(), "sites.json")
	if _, err := os.Stat(filename); err != nil {
		jsonObj := gabs.New()
		ioutil.WriteFile(filename, []byte(jsonObj.String()), 666)
	}
	return utils.LoadJSON(filename)
}
