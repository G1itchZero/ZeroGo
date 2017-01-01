package downloader

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/G1itchZero/ZeroGo/events"
	"github.com/G1itchZero/ZeroGo/interfaces"
	"github.com/G1itchZero/ZeroGo/peer_manager"
	"github.com/G1itchZero/ZeroGo/tasks"
	"github.com/G1itchZero/ZeroGo/utils"
	"github.com/Jeffail/gabs"
	"github.com/fatih/color"

	log "github.com/Sirupsen/logrus"
)

type FilterFunc func(string) bool

type Downloader struct {
	Address          string
	Peers            *peer_manager.PeerManager
	Tasks            tasks.Tasks
	Files            map[string]*tasks.FileTask
	ContentRequested bool
	Content          *gabs.Container
	TotalFiles       int
	StartedTasks     int
	OnChanges        chan events.SiteEvent
	sync.Mutex
}

func NewDownloader(address string) *Downloader {
	d := Downloader{
		Peers:        peer_manager.NewPeerManager(address),
		Address:      address,
		OnChanges:    make(chan events.SiteEvent, 400),
		Files:        map[string]*tasks.FileTask{},
		StartedTasks: 0,
	}
	return &d
}

func (d *Downloader) Download(done chan int, filter FilterFunc, modified float64) bool {
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

	dir := path.Join(utils.GetDataPath(), d.Address)
	os.MkdirAll(dir, 0777)

	d.ContentRequested = false
	d.Tasks = tasks.Tasks{tasks.NewTask("content.json", "", 0, d.Address, d.OnChanges)}

	go d.Peers.Announce()
	d.processContent(filter)
	d.ContentRequested = true
	if d.Content.S("modified").Data().(float64) == modified {
		log.Println(fmt.Sprintf("Not modified: %v", modified))
		for _, task := range d.Tasks {
			task.Done = true
		}
		done <- 0
		return true
	}
	log.Println(fmt.Sprintf("Files in queue: %s", green(len(d.Tasks)-1)))
	sort.Sort(d.Tasks)
	for _, task := range d.Tasks {
		go d.ScheduleFile(task)
	}
	for d.PendingTasksCount() > 0 {
		select {
		case p := <-d.Peers.OnPeers:
			sort.Sort(d.Tasks)
			n := 0
			t := d.Tasks[n]
			for t.Done {
				n++
				if n >= len(d.Tasks) {
					n = -1
					break
				}
				t = d.Tasks[n]
			}
			if n >= 0 {
				log.WithFields(log.Fields{
					"task": t,
					"peer": p,
				}).Debug("New new peer ->")
				go d.ScheduleFileForPeer(t, p)
			}
		}
	}
	done <- 0
	return true
}

func (d *Downloader) GetContent() (*gabs.Container, error) {
	filename := path.Join(utils.GetDataPath(), d.Address, "content.json")
	if _, err := os.Stat(filename); err != nil {
		fmt.Println("No downloaded content.json")
		return nil, errors.New("Not downloaded yet")
	}
	return utils.LoadJSON(filename)
}

func (d *Downloader) processContent(filter FilterFunc) *tasks.FileTask {
	d.ContentRequested = true
	task := d.ScheduleFile(d.Tasks[0])
	content, _ := gabs.ParseJSON(task.Content)
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	includes, _ := content.S("includes").ChildrenMap()
	if includes != nil {
		for k := range includes {
			info, _ := gabs.Consume(map[string]interface{}{
				"sha512": "", "size": 1024000.0,
			})
			files[k] = info
		}
	}
	for filename, child := range files {
		if filter != nil && !filter(filename) {
			continue
		}
		file := child.Data().(map[string]interface{})
		t := tasks.NewTask(filename, file["sha512"].(string), file["size"].(float64), d.Address, d.OnChanges)
		d.Tasks = append(d.Tasks, t)
		d.Files[t.Filename] = t
		log.WithFields(log.Fields{
			"task": task,
		}).Debug("New task")
	}
	d.TotalFiles = len(files) + 1 //content.json
	return task

}

func (d *Downloader) ScheduleFileForPeer(task *tasks.FileTask, peer interfaces.IPeer) *tasks.FileTask {
	// filename := path.Join(utils.GetDataPath(), d.Address, task.Filename)
	// if _, err := os.Stat(filename); err == nil && task.Filename != "content.json" {
	// 	log.WithFields(log.Fields{
	// 		"task": task.Filename,
	// 	}).Info("File from disk")
	// 	task.Start()
	// 	task.Finish()
	// 	return task
	// }
	log.WithFields(log.Fields{
		"task": task.Filename,
		"peer": peer.GetAddress(),
	}).Info("Requesting file")
	task.AddPeer(peer)
	return task
}

func (d *Downloader) ScheduleFile(task *tasks.FileTask) *tasks.FileTask {
	d.StartedTasks++
	if d.PendingTasksCount() == 0 {
		return nil
	}
	peer := d.Peers.Get()

	for peer == nil {
		peer = d.Peers.Get()
		time.Sleep(100)
	}
	return d.ScheduleFileForPeer(task, peer)
}

func (d *Downloader) PendingTasksCount() int {
	n := 0
	for _, task := range d.Tasks {
		if !task.Done {
			n++
		}
	}
	return n
}

func (d *Downloader) PendingTasks() tasks.Tasks {
	res := tasks.Tasks{}
	for _, task := range d.Tasks {
		if !task.Done {
			res = append(res, task)
		}
	}
	return res
}

func (d *Downloader) FinishedTasks() int {
	n := 0
	for _, task := range d.Tasks {
		if task.Done {
			n++
		}
	}
	return n
}
