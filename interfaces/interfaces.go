package interfaces

type ISite interface {
}

type ITask interface {
	GetSite() string
	GetFilename() string
	GetContent() []byte
	SetContent([]byte)
	Start()
	Finish()
}
type IPeer interface {
	AddTask(ITask)
	GetAddress() string
}
