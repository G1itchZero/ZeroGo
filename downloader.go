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
	Site       string
	Filename   string
	Hash       string  `json:"sha512"`
	Size       float64 `json:"size"`
	Downloaded float64
	Content    []byte
	Peers      int
}
type Tasks []*FileTask

type Downloader struct {
	Address          string
	Peers            *PeerManager
	Tasks            Tasks
	ContentRequested bool
	Content          *gabs.Container
	TotalFiles       int
	StartedTasks     int
	OnChanges        chan SiteEvent
	InProgress       bool
	tasksDone        chan *FileTask
	trackersDone     int
	filesDone        int
	done             chan int
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
		Peers:        NewPeerManager(address),
		Address:      address,
		OnChanges:    make(chan SiteEvent, 400),
		tasksDone:    make(chan *FileTask, 100),
		trackersDone: 0,
		filesDone:    1,
		StartedTasks: 0,
	}
	return &d
}

func (d *Downloader) handleFile(file *FileTask) {
	if file == nil {
		return
	}
	d.filesDone++
	d.OnChanges <- SiteEvent{"file_done", file.Filename}
	log.WithFields(log.Fields{
		"file": file.Filename,
	}).Infof("Task completed [%d/%d] in q: %d", d.filesDone, d.TotalFiles, d.PendingTasks())

	go func() {
		pt := d.PendingTasks()
		done := d.FinishedTasks()
		fmt.Println(done, d.TotalFiles, pt)
		if pt > 0 {
			d.tasksDone <- d.scheduleFile(d.Address)
		} else if done >= d.TotalFiles {
			d.InProgress = false
			d.done <- 0
			go d.Peers.Stop()
		}
	}()
}

func (d *Downloader) handlePeer(peer *Peer) {
	if !d.ContentRequested {
		d.tasksDone <- d.processContent()
	}
}

func (d *Downloader) handleAnnounce(peers int) {
	//TODO: waiting for connections
	go func() { d.OnChanges <- SiteEvent{"peers_added", peers} }()
	d.trackersDone++
	if d.trackersDone == len(d.Peers.Trackers) && d.Peers.Count == 0 {
		fmt.Println("No peers founded.")
		d.InProgress = false
		return
	}
}

func (d *Downloader) Download(done chan int) bool {
	success := true
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

	dir := path.Join(DATA, d.Address)
	os.MkdirAll(dir, 0777)

	d.ContentRequested = false
	d.Tasks = Tasks{&FileTask{
		Filename: "content.json",
		Site:     d.Address,
	}}

	d.done = make(chan int, 100)
	go d.Peers.Announce()
	d.InProgress = true
	for d.InProgress {
		select {
		case file := <-d.tasksDone:
			go d.handleFile(file)
		case peer := <-d.Peers.OnPeers:
			go d.handlePeer(peer)
		case peers := <-d.Peers.OnAnnounce:
			go d.handleAnnounce(peers)
		case <-d.done:
			break
		}
	}
	fmt.Println(green("Site downloaded."))
	d.filesDone = 0
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

func (d *Downloader) processContent() *FileTask {
	d.ContentRequested = true
	task := d.scheduleFile(d.Address)
	content, _ := gabs.ParseJSON(task.Content)
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	for filename, child := range files {
		file := child.Data().(map[string]interface{})
		d.Tasks = append(d.Tasks, &FileTask{
			Filename: filename,
			Hash:     file["sha512"].(string),
			Size:     file["size"].(float64),
			Site:     d.Address,
		})
	}
	d.TotalFiles = len(files) + 1 //content.json
	return task

}

func (d *Downloader) scheduleFile(site string) *FileTask {
	d.StartedTasks++
	if d.PendingTasks() == 0 {
		return nil
	}
	peer := d.Peers.Get()
	if peer == nil {
		return nil
	}

	for _, task := range d.Tasks {
		if len(task.Content) == 0 {
			filename := path.Join(DATA, site, task.Filename)
			task.Peers++
			if _, err := os.Stat(filename); err == nil && task.Filename != "content.json" {
				log.WithFields(log.Fields{
					"task": task.Filename,
				}).Info("File from disk")
				task.Peers--
				return task
			}
			log.WithFields(log.Fields{
				"task": task.Filename,
				"peer": peer.Address,
			}).Info("Requesting file")
			peer.Download(task)
			task.Peers--
			return task
		}
	}
	return nil
}

func (d *Downloader) PendingTasks() int {
	n := 0
	for _, task := range d.Tasks {
		if len(task.Content) == 0 {
			n++
		}
	}
	return n
}

func (d *Downloader) FinishedTasks() int {
	n := 0
	for _, task := range d.Tasks {
		if len(task.Content) != 0 {
			n++
		}
	}
	return n
}
