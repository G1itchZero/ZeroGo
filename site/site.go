package site

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"

	"github.com/G1itchZero/zeronet-go/downloader"
	"github.com/G1itchZero/zeronet-go/events"
	"github.com/G1itchZero/zeronet-go/utils"
	"github.com/Jeffail/gabs"
	log "github.com/Sirupsen/logrus"
)

type Site struct {
	Address    string
	Path       string
	Content    *gabs.Container
	Done       chan *Site
	Downloader *downloader.Downloader
	Added      int
	Ready      bool
	Success    bool
	OnChanges  chan events.SiteEvent
	Filter     downloader.FilterFunc
	sync.Mutex
}

func NewSite(address string) *Site {
	done := make(chan *Site, 2)
	site := Site{
		Address:    address,
		Path:       path.Join(utils.GetDataPath(), address),
		Done:       done,
		Downloader: downloader.NewDownloader(address),
		Ready:      false,
		Success:    false,
	}
	// site.OnChanges = site.Downloader.OnChanges
	// go func() {
	// 	for {
	// 		select {
	// 		case event := <-site.Downloader.OnChanges:
	// 			log.Fatal(event)
	// 		}
	// 	}
	// }()
	site.Content, _ = site.Downloader.GetContent()
	return &site
}

func (site *Site) Download(ch chan *Site) {
	site.Done = ch
	if site.Downloader.TotalFiles != 0 && site.Downloader.FinishedTasks() == site.Downloader.TotalFiles {
		site.Ready = true
		site.Done <- site
		return
	}
	done := make(chan int)
	go func() {
		site.Lock()
		site.Success = site.Downloader.Download(done, site.Filter)
		go site.handleEvents()
		site.Unlock()
	}()
	<-done
	site.Content = site.Downloader.Content
	site.Ready = true
	site.Done <- site
}

func (site *Site) handleEvents() {
	for {
		select {
		case peersCount := <-site.Downloader.Peers.OnAnnounce:
			site.OnChanges <- events.SiteEvent{Type: "peers_added", Payload: peersCount}
		}
	}
}

func (site *Site) GetFile(filename string) ([]byte, error) {
	content, err := ioutil.ReadFile(path.Join(site.Path, filename))
	if err == nil {
		return content, nil
	}
	return nil, err
}

func (site *Site) Remove() {
	err := os.RemoveAll(site.Path)
	if err != nil {
		log.WithFields(log.Fields{
			"site": site.Path,
			"err":  err,
		}).Error("Error during site removing")
	}
}

func (site *Site) Wait() {
	for site.Downloader.PendingTasksCount() > 0 {
		fmt.Println("Files left:", site.Downloader.PendingTasksCount())
		time.Sleep(time.Millisecond * 100)
	}
}

func (site *Site) WaitFile(filename string) {
	task, ok := site.Downloader.Files[filename]
	for !ok {
		task, ok = site.Downloader.Files[filename]
		// fmt.Println("waiting for", task, filename, ok, site.Downloader.Files)
		time.Sleep(time.Duration(time.Millisecond * 100))
	}
	n := 0
	for !task.Done {
		fmt.Println("waiting for", task)
		log.WithFields(log.Fields{
			"task": task,
		}).Info("Waiting for file")
		time.Sleep(time.Duration(time.Millisecond * 100))
		n++
		task.Priority++
		if n > 20 {
			go site.Downloader.ScheduleFile(task)
			n = 0
		}
	}
}

func (site *Site) GetSettings() SiteSettings {
	size := 0.0
	modified := 0.0

	if site.Content != nil {
		modified = site.Content.Path("modified").Data().(float64)
		files, _ := site.Content.S("files").ChildrenMap()
		for _, file := range files {
			size += file.Path("size").Data().(float64)
		}
	}
	return SiteSettings{

		Added:              site.Added,
		BytesRecv:          size,
		OptionalDownloaded: 0,
		BytesSent:          0,
		Peers:              site.Downloader.Peers.Count,
		Modified:           modified,
		SizeOptional:       0,
		Serving:            false,
		Own:                false,
		Permissions:        []string{"ADMIN"},
		Size:               size,
	}
}

func (site *Site) GetInfo() SiteInfo {
	var content interface{}
	content = nil
	if site.Content != nil {
		content = site.Content.Data()
	}
	return SiteInfo{
		Address:  site.Address,
		Files:    len(site.Downloader.Tasks) - 1,
		Peers:    site.Downloader.Peers.Count,
		Content:  content,
		Workers:  len(site.Downloader.Peers.GetActivePeers()),
		Tasks:    len(site.Downloader.Peers.GetActivePeers()),
		Settings: site.GetSettings(),

		SizeLimit:     100,
		NextSizeLimit: 120,
		AuthAddress:   "",
		// AuthKeySha512:  "",
		// AuthKey:        "",
		BadFiles:       0,
		StartedTaskNum: site.Downloader.StartedTasks,
		ContentUpdated: 0,
	}
}

type SiteInfo struct {
	Address       string       `json:"address"`
	Files         int          `json:"files"`
	Peers         int          `json:"peers"`
	Content       interface{}  `json:"content"`
	Workers       int          `json:"workers"`
	Tasks         int          `json:"tasks"`
	Settings      SiteSettings `json:"settings"`
	SizeLimit     int          `json:"size_limit"`
	NextSizeLimit int          `json:"next_size_limit"`
	AuthAddress   string       `json:"auth_address"`
	// AuthKeySha512  string       `json:"auth_key_sha512"`
	// AuthKey        string       `json:"auth_key"`
	BadFiles       int           `json:"bad_files"`
	CertUserID     interface{}   `json:"cert_user_id"`
	StartedTaskNum int           `json:"started_task_num"`
	ContentUpdated float64       `json:"content_updated"`
	Event          []interface{} `json:"event"`
}

type SiteSettings struct {
	Added              int     `json:"added"`
	BytesRecv          float64 `json:"bytes_recv"`
	OptionalDownloaded int     `json:"optional_downloaded"`
	Cache              struct {
	} `json:"cache"`
	BytesSent    int      `json:"bytes_sent"`
	Peers        int      `json:"peers"`
	Modified     float64  `json:"modified"`
	SizeOptional int      `json:"size_optional"`
	Serving      bool     `json:"serving"`
	Own          bool     `json:"own"`
	Permissions  []string `json:"permissions"`
	Size         float64  `json:"size"`
}

// type AutoGenerated struct {
// 	To     int    `json:"to"`
// 	Cmd    string `json:"cmd"`
// 	Result []struct {
// 		Content struct {
// 			Files                    int     `json:"files"`
// 			Description              string  `json:"description"`
// 			ClonedFrom               string  `json:"cloned_from"`
// 			Address                  string  `json:"address"`
// 			Includes                 int     `json:"includes"`
// 			Cloneable                bool    `json:"cloneable"`
// 			Optional                 string  `json:"optional"`
// 			InnerPath                string  `json:"inner_path"`
// 			Title                    string  `json:"title"`
// 			FilesOptional            int     `json:"files_optional"`
// 			SignsRequired            int     `json:"signs_required"`
// 			Modified                 float64 `json:"modified"`
// 			Ignore                   string  `json:"ignore"`
// 			ZeronetVersion           string  `json:"zeronet_version"`
// 			PostmessageNonceSecurity bool    `json:"postmessage_nonce_security"`
// 			AddressIndex             int     `json:"address_index"`
// 			BackgroundColor          string  `json:"background-color"`
// 		} `json:"content"`
// 	} `json:"result"`
// 	ID int `json:"id"`
// }
