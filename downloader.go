package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/Jeffail/gabs"
	"github.com/fatih/color"

	log "github.com/Sirupsen/logrus"
)

type FileTask struct {
	Filename string
	Hash     string  `json:"sha512"`
	Size     float64 `json:"size"`
	Content  []byte
}
type Tasks []FileTask

type Downloader struct {
	Address          string
	Peers            *PeerManager
	Queue            Tasks
	Done             Tasks
	ContentRequested bool
	Content          *gabs.Container
	TotalFiles       int
	OnChanges        chan SiteEvent
	InProgress       bool
	sync.Mutex
}

type Announce struct {
	InfoHash   string `url:"info_hash"`
	PeerID     string `url:"peer_id"`
	Port       int    `url:"port"`
	Uploaded   int    `url:"uploaded"`
	Downloaded int    `url:"downloaded"`
	Left       int    `url:"left"`
	Compact    int    `url:"compact"`
	NumWant    int    `url:"numwant"`
	Event      string `url:"event"`
}

func NewDownloader(address string) *Downloader {
	d := Downloader{
		Peers:     NewPeerManager(address),
		Address:   address,
		OnChanges: make(chan SiteEvent, 400),
	}
	return &d
}

func (d *Downloader) Download(done chan int) bool {
	success := true
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

	dir := path.Join(DATA, d.Address)
	os.MkdirAll(dir, 0777)

	d.ContentRequested = false
	d.Queue = []FileTask{FileTask{
		Filename: "content.json",
	}}

	files := make(chan FileTask, 100)
	inbox := d.Peers.OnPeers
	announce := d.Peers.OnAnnounce
	go d.Peers.Announce()
	// announced := false
	tCount := 0
	fCount := 1
	d.InProgress = true
	for d.InProgress {
		select {
		case file := <-files:
			if file.Filename == "" {
				break
			}
			fCount++
			d.OnChanges <- SiteEvent{"file_done", file.Filename}
			log.WithFields(log.Fields{
				"file": file.Filename,
			}).Infof("Task completed [%d/%d] in q: %d", fCount, len(d.Queue)+len(d.Done), len(d.Queue))
			// log.Fatal("")
			if len(d.Queue) > 0 {
				go func() {
					d.Lock()
					files <- d.schedileFile(d.Address)
					d.Unlock()
				}()
			} else if len(d.Done) == d.TotalFiles {
				fmt.Println("Site downloaded.")
				d.InProgress = false
				break
			}
		case peer := <-inbox:
			// go func() { d.OnChanges <- 0 }()
			go func(peer *Peer) {
				if !d.ContentRequested {
					d.Lock()
					files <- d.processContent()
					d.Unlock()
				} else {
					d.Lock()
					if len(d.Queue) > 0 {
						files <- d.schedileFile(d.Address)
					} else {
						// fmt.Println("All files scheduled")
					}
					d.Unlock()
				}
			}(peer)
		case c := <-announce:
			//TODO: waiting for connections
			go func() { d.OnChanges <- SiteEvent{"peers_added", c} }()
			tCount++
			if tCount == len(d.Peers.Trackers) && d.Peers.Count == 0 {
				fmt.Println("No peers founded.")
				d.InProgress = false
				break
			}
		}
	}
	fmt.Println("Downloader finished.")
	done <- 0
	return success
	// fmt.Printf("Peers: %s", yellow(len(d.Peers)))
}

func (d *Downloader) GetContent() (*gabs.Container, error) {
	filename := path.Join(DATA, d.Address, "content.json")
	if _, err := os.Stat(filename); err != nil {
		return nil, errors.New("Not downloaded yet")
	}
	return loadJSON(filename)
}

func (d *Downloader) processContent() FileTask {
	d.ContentRequested = true
	task := d.schedileFile(d.Address)
	content, _ := gabs.ParseJSON(task.Content)
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	for filename, child := range files {
		file := child.Data().(map[string]interface{})
		d.Queue = append(d.Queue, FileTask{
			Filename: filename,
			Hash:     file["sha512"].(string),
			Size:     file["size"].(float64),
		})
	}
	d.TotalFiles = len(files) + 1
	return task

}

func (d *Downloader) schedileFile(site string) FileTask {
	if len(d.Queue) == 0 {
		return FileTask{}
	}
	peer := d.Peers.Get()
	if peer == nil {
		return FileTask{}
	}
	task := d.Queue[0]
	filename := path.Join(DATA, site, task.Filename)
	if _, err := os.Stat(filename); err == nil && task.Filename != "content.json" {
		d.Queue = d.Queue[1:]
		d.Done = append(d.Done, task)
		return task
	}
	log.WithFields(log.Fields{
		"task": task,
		"peer": peer,
	}).Info("Requesting file")
	d.Queue = d.Queue[1:]
	file := peer.Download(site, task.Filename)
	d.Done = append(d.Done, task)
	d.Peers.Free(peer)
	task.Content = file
	return task
}
