package interfaces

type ISite interface {
}

type ITask interface {
	GetSite() string
	GetFilename() string
	GetContent() []byte
	GetStarted() bool
	SetContent([]byte)
	AppendContent([]byte, int)
	GetSize() int64
	Start()
	Finish()
}
type IPeer interface {
	AddTask(ITask) error
	GetAddress() string
}
