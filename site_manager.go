package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/Jeffail/gabs"
)

type SiteManager struct {
	Sites map[string]*Site
}

func NewSiteManager() *SiteManager {
	sm := SiteManager{
		Sites: map[string]*Site{},
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

func (sm *SiteManager) Get(address string) *Site {
	site, ok := sm.Sites[address]
	if !ok {
		site = NewSite(address, sm)
		site.Added = int(time.Now().Unix())
		sm.Sites[address] = site
	}
	go sm.processSite(site)
	return site
}

func (sm *SiteManager) processSite(site *Site) {
	done := make(chan *Site, 2)
	site.Download(done)
	site.Wait()
	sm.SaveSites()
}

func (sm *SiteManager) SaveSites() {
	sites := sm.GetSites()
	// log.Fatal(sites)
	filename := path.Join(DATA, "sites.json")
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
			fmt.Println("preload", address)
			sm.Sites[address] = NewSite(address, sm)
		}
	}
}

func loadSites() (*gabs.Container, error) {
	filename := path.Join(DATA, "sites.json")
	if _, err := os.Stat(filename); err != nil {
		jsonObj := gabs.New()
		ioutil.WriteFile(filename, []byte(jsonObj.String()), 644)
	}
	return loadJSON(filename)
}
