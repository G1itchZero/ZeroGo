package server

import (
	"bytes"
	"html/template"
	"log"
	"fmt"

	"github.com/G1itchZero/ZeroGo/site"
	"github.com/G1itchZero/ZeroGo/utils"
	"github.com/labstack/echo"
)

const TEMPLATE = `
<!DOCTYPE html>

<html>
<head>
 <title>{{.Title}} - ZeroNet</title>
 <meta charset="utf-8" />
 <meta http-equiv="content-type" content="text/html; charset=utf-8" />
 <link rel="stylesheet" href="/uimedia/all.css?rev={{.Rev}}" />
 {{.MetaTags}}
</head>
<body style="{{.BodyStyle}}">

<script>
// If we are inside iframe escape from it
if (window.self !== window.top) window.open(window.location.toString(), "_top");
if (window.self !== window.top) window.stop();
if (window.self !== window.top && document.execCommand) document.execCommand("Stop", false)


// Dont allow site to load in a popup
/*
if (window.opener) document.write("Opener not allowed")
if (window.opener && document.execCommand) document.execCommand("Stop", false)
if (window.opener && window.stop) window.stop()
*/
</script>

<div class="progressbar">
 <div class="peg"></div>
</div>

<!-- Fixed button -->
<div class='fixbutton'>
 <div class='fixbutton-text'>0</div>
 <div class='fixbutton-burger'>&#x2261;</div>
 <a class='fixbutton-bg' href="{{.Homepage}}/"></a>
</div>


<!-- Notifications -->
<div class='notifications'>
 <div class='notification template'><span class='notification-icon'>!</span> <span class='body'>Test notification</span><a class="close" href="#Close">&times;</a><div style="clear: both"></div></div>
</div>


<!-- Loadingscreen -->
<div class='loadingscreen'>
 <div class='loading-text console'>
 </div>
 <div class="flipper-container">
  <div class="flipper"> <div class="front"></div><div class="back"></div> </div>
 </div>
</div>


<!-- Site Iframe -->
<iframe src='about:blank' id='inner-iframe' sandbox="allow-forms allow-scripts allow-top-navigation allow-popups {{.SandboxPermissions}}"></iframe>

<!-- Site info -->
<script>
document.getElementById("inner-iframe").src = "about:blank"
document.getElementById("inner-iframe").src = "{{.FileURL}}{{.QueryString}}"
address = "{{.Address}}"
wrapper_nonce = "{{.Nonce}}"
wrapper_key = "{{.Key}}"
postmessage_nonce_security = {{.PostmessageNonceSecurity}}
file_inner_path = "{{.FileInnerPath}}"
permissions = ["ADMIN"]
show_loadingscreen = {{.ShowLoadingScreen}}
server_url = '{{.ServerURL}}'

if (typeof WebSocket === "undefined")
	document.body.innerHTML += "<div class='unsupported'>Your browser is not supported please use <a href='http://outdatedbrowser.com'>Chrome or Firefox</a>.</div>";
</script>
<script type="text/javascript" src="/uimedia/all.js?rev={{.Rev}}&lang={{.Lang}}"></script>

</body>
</html>
`

type Wrapper struct {
	Nonce                    string
	Key                      string
	Title                    string
	Rev                      int
	FileURL                  string
	Address                  string
	FileInnerPath            string
	ShowLoadingScreen        bool
	Lang                     string
	QueryString              string
	Homepage                 string
	MetaTags                 string
	BodyStyle                string
	SandboxPermissions       string
	PostmessageNonceSecurity string
	ServerURL                string
}

func NewWrapper(s *site.Site) *Wrapper {
	nonce := utils.RandomString(36)
	wrapperKey := utils.RandomString(36)
	title := s.Address
	if s.Content != nil {
		title = s.Content.S("title").Data().(string)
	}
	w := Wrapper{
		Rev:               utils.REV,
		Title:             title,
		FileURL:           "/inner/" + s.Address,
		Address:           s.Address,
		FileInnerPath:     "index.html",
		ShowLoadingScreen: true,
		Lang:              "en",
		QueryString:       "?wrapper_nonce=" + nonce,
		Nonce:             nonce,
		Key:               wrapperKey,
		Homepage:          "/" + utils.ZN_HOMEPAGE,
	}
	return &w
}

func (w *Wrapper) Render(ctx echo.Context) error {
	tmpl, _ := template.New("wrapper").Parse(TEMPLATE)
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, w); err != nil {
		return err
	}
	ctx.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	ctx.Response().WriteHeader(200)
	_, err := ctx.Response().Write(buf.Bytes())
	if err != nil {
		log.Fatal(fmt.Errorf("Render error: %v", err))
	}
	return err
}
