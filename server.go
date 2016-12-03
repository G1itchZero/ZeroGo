package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
)

type Server struct {
	Port    int
	Sockets map[string]*UiSocket
	Sites   *SiteManager
}

func NewServer(port int, sites *SiteManager) *Server {
	server := Server{
		Port:    port,
		Sockets: map[string]*UiSocket{},
		Sites:   sites,
	}
	return &server
}

func InnerMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		if ctx.Path() == "/inner/:site" {
			site := ctx.Param("site")
			cookie := new(http.Cookie)
			cookie.Name = "site"
			cookie.Value = site
			cookie.Expires = time.Now().Add(24 * time.Hour)
			ctx.SetCookie(cookie)
		}
		return next(ctx)
	}
}

var (
	upgrader = websocket.Upgrader{}
)

func (s *Server) socketHandler(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	wrapperKey := c.QueryParam("wrapper_key")
	if err != nil {
		return err
	}
	defer ws.Close()

	socket, ok := s.Sockets[wrapperKey]

	if ws != nil && ok {
		socket.Serve(ws)
	}
	return nil
}

func (s *Server) Serve() {
	e := echo.New()
	t := &Template{
		templates: template.Must(template.ParseGlob("./template/*.html")),
	}
	e.Renderer = t

	e.Static("/uimedia", "./media")

	inner := e.Group("/inner")
	inner.Use(InnerMiddleware)
	inner.GET("/:site", s.serveInner)
	inner.GET("/*", s.serveInnerStatic)

	e.GET("/Websocket", s.socketHandler)
	e.GET("/:url/", s.serveWrapper)
	e.GET("/:url", s.serveWrapper)
	e.GET("/", s.serveWrapper)

	e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", s.Port)))
}

func (s *Server) serveWrapper(ctx echo.Context) error {
	yellow := color.New(color.FgYellow).SprintFunc()
	url := ctx.Param("url")
	if url == "" {
		url = ZN_HOMEPAGE
	}
	done := s.Sites.Get(url)
	fmt.Println(fmt.Sprintf("> %s", yellow(url)))
	nonce := randomString(36)
	wrapperKey := randomString(36)
	site := <-done
	err := ctx.Render(http.StatusOK, "wrapper", map[string]interface{}{
		"rev":                REV,
		"title":              "Hello!",
		"file_url":           "/inner/" + site.Address,
		"address":            site.Address,
		"file_inner_path":    "index.html",
		"show_loadingscreen": true,
		// "permissions":        []string{"ADMIN"},
		"lang":          "en",
		"query_string":  "?wrapper_nonce=" + nonce,
		"wrapper_nonce": nonce,
		"wrapper_key":   wrapperKey,
	})
	socket := NewUiSocket(site, wrapperKey)
	s.Sockets[wrapperKey] = socket
	return err
}

func (s *Server) serveInner(ctx echo.Context) error {
	site := ctx.Param("site")
	root := path.Join(DATA, site)
	filename := path.Join(root, "index.html")
	return ctx.File(filename)
}

func (s *Server) serveInnerStatic(ctx echo.Context) error {
	site, _ := ctx.Cookie("site")
	url := ctx.Param("*")
	root := path.Join(DATA, site.Value)
	filename := path.Join(root, url)
	return ctx.File(filename)
}

func serveStatic(ctx echo.Context, p string) error {
	fmt.Println(p)
	return ctx.File(p)
	// if strings.HasSuffix(p, ".css") {
	// 	ctx.SetHeader("Content-Type", "text/css")
	// }
	// if strings.HasSuffix(p, ".js") {
	// 	ctx.SetHeader("Content-Type", "text/javascript")
	// }
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}
