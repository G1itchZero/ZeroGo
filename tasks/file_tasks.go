package tasks

import "github.com/G1itchZero/zeronet-go/interfaces"

type Tasks []*FileTask
type FileTask struct {
	Site       string
	Filename   string
	Hash       string  `json:"sha512"`
	Size       float64 `json:"size"`
	Downloaded float64
	Content    []byte
	Peers      []interfaces.IPeer
}

func (task *FileTask) GetFilename() string {
	return task.Filename
}

func (task *FileTask) GetContent() []byte {
	return task.Content
}

func (task *FileTask) SetContent(content []byte) {
	task.Content = content
}

func (task *FileTask) GetSite() string {
	return task.Site
}

func (task *FileTask) AddPeer(p interfaces.IPeer) {
	if task.Peers == nil {
		task.Peers = []interfaces.IPeer{}
	}
	task.Peers = append(task.Peers, p)
	p.AddTask(task)
}
