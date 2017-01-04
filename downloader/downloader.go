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
	"gopkg.in/cheggaaa/pb.v1"

	log "github.com/Sirupsen/logrus"
)

type FilterFunc func(string) bool

type Downloader struct {
	Address          string
	Peers            *peer_manager.PeerManager
	Tasks            tasks.Tasks
	Files            map[string]*tasks.FileTask
	Includes         []string
	ContentRequested bool
	Content          *gabs.Container
	TotalFiles       int
	StartedTasks     int
	OnChanges        chan events.SiteEvent
	ProgressBar      *pb.ProgressBar
	sync.Mutex
}

func NewDownloader(address string) *Downloader {
	green := color.New(color.FgGreen).SprintFunc()
	d := Downloader{
		Peers:        peer_manager.NewPeerManager(address),
		Address:      address,
		OnChanges:    make(chan events.SiteEvent, 400),
		Files:        map[string]*tasks.FileTask{},
		StartedTasks: 0,
		Includes:     []string{},
	}
	if !utils.GetDebug() {
		d.ProgressBar = pb.New(1).Prefix(green(address))
		d.ProgressBar.SetRefreshRate(time.Millisecond * 50)
		d.ProgressBar.ShowTimeLeft = false
	}
	return &d
}

func (d *Downloader) Download(done chan int, filter FilterFunc, modified float64) bool {
	green := color.New(color.FgGreen).SprintFunc()
	// fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

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
		log.Info("No downloaded content.json")
		return nil, errors.New("Not downloaded yet")
	}
	return utils.LoadJSON(filename)
}

func (d *Downloader) processContent(filter FilterFunc) *tasks.FileTask {
	d.ContentRequested = true
	task := d.ScheduleFile(d.Tasks[0])
	content, _ := gabs.ParseJSON(task.GetContent())
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	includes, _ := content.S("includes").ChildrenMap()
	if includes != nil {
		for name := range includes {
			info, _ := gabs.Consume(map[string]interface{}{
				"sha512": "", "size": 2048000.0,
			})
			files[name] = info
			d.Includes = append(d.Includes, name)
			// func(name string) {
			t := tasks.NewTask(name, "", 2048000.0, d.Address, d.OnChanges)
			d.Tasks = append(d.Tasks, t)
			t = d.ScheduleFile(t)
			// log.Fatal(t)
			content, err := gabs.ParseJSON(t.GetContent())
			if err != nil {
				log.Fatalf("Include content error: %s", err)
			}
			users, err := content.S("user_contents").S("archived").ChildrenMap()
			if err != nil {
				// log.Fatalf("Include users list error: %s", err)
				continue
			}
			for u := range users {
				u = path.Join("data/users", u)
				info, _ := gabs.Consume(map[string]interface{}{
					"sha512": "", "size": 2048000.0,
				})
				files[path.Join(u, "content.json")] = info
				files[path.Join(u, "data.json")] = info
			}
			// }(k)
		}
	}
	d.TotalFiles = len(files) + 1 //content.json
	for filename, child := range files {
		if filter != nil && !filter(filename) {
			d.TotalFiles--
			continue
		}
		file := child.Data().(map[string]interface{})
		t := tasks.NewTask(filename, file["sha512"].(string), file["size"].(float64), d.Address, d.OnChanges)
		log.Println(filename)
		d.Tasks = append(d.Tasks, t)
		d.Files[t.Filename] = t
		log.WithFields(log.Fields{
			"task": task,
		}).Debug("New task")
	}
	if d.ProgressBar != nil {
		d.ProgressBar.Total = int64(d.TotalFiles)
	}
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
	res := task.AddPeer(peer)
	if d.ProgressBar != nil && res == nil {
		d.ProgressBar.Increment()
		d.ProgressBar.Update()
	}
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
