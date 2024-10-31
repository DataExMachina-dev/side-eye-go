package sideeye

import (
	"context"
	"fmt"
	"github.com/DataExMachina-dev/side-eye-go/internal/sideeyeconn"
	"html/template"
	"net/http"
	"time"
)

var configPageTemplate = template.Must(template.
	New("config-page").
	Funcs(template.FuncMap{}).
	Parse(`
<html>
<head>
	<title>Side-Eye configuration</title>
	<style>
	.circle {
		height: 21px;
		width: 21px;
		border-radius: 50%;
		display: inline-block;
	}
	</style>
</head>
<body>
<h1>Side-Eye configuration</h1>
{{if .ErrMsg}}
<p>
	<span style="color:red">{{.ErrMsg}}</span>
</p>
{{end}}
<form action="" method="POST">
	<div style="
		display:grid;
		gap:3px;
		grid-template-columns: 9em 20em;
		margin-bottom: 10px;"
		>
		<div>Connection status:</div>
		<div style="display:flex; flex-direction:row; align-items:center; gap:3px">
			<div class="circle" style="background-color:{{.ConnColor}};"></div>
			<span>{{.ConnStatus}}</span>
		</div>
		<div>Side-Eye token:</div>
		<input type="text" name="token" value="{{.Token}}"/>
		<div>Environment:</div>
		<input type="text" name="env" value="{{.Env}}"/>
		<div>Program name:</div>
		<input type="text" name="programName" value="{{.Program}}"/>
	</div>
	<input type="submit" value="Reconnect" name="connect"/>
	<input type="submit" value="Disconnect" name="disconnect" {{if not .DisconnectEnabled}} disabled {{end}}/>
</form>
</body>
</html>
`))

// HttpHandler is a handler that renders the Side-Eye configuration page. This
// page lets a user connect or disconnect from the Side-Eye cloud service.
//
// Example usage:
// http.Handle("/sideeye", sideeye.HttpHandler())
//
// The options dictate the default configuration values for the connection to
// Side-Eye. If Init() was called to connect to the Side-Eye cloud service, then
// no options should be passed here; the options passed to Init() will be used
// automatically. Note that calling HttpHandler() does not automatically
// establish a connection to Side-Eye. However, the web page served by this
// handler can be used to establish a connection manually.
func HttpHandler(opts ...Option) http.Handler {
	var cfg sideeyeconn.Config
	if singletonConn.Status() == sideeyeconn.Uninitialized {
		cfg = sideeyeconn.MakeDefaultConfig("" /* programName */)
	} else {
		cfg = singletonConn.ActiveConfig
	}
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	return &httpHandler{
		config: cfg,
	}
}

type httpHandler struct {
	// The config that the configuration page updates.
	config sideeyeconn.Config
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// GETs renders the current state of the connection.
	if req.Method == http.MethodGet {
		h.handleGet(w, "" /* errMsg */)
		return
	}

	// If this is a POST request, stop the old connection (if any) and, if a token
	// is specified, start a new connection using it.
	if err := req.ParseForm(); err != nil {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("failed to parse form: %w", err))
		return
	}

	if _, ok := req.Form["disconnect"]; ok {
		Stop()
		h.handleGet(w, "" /* errMsg */)
		return
	}

	if _, ok := req.Form["connect"]; !ok {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("invalid POST: missing connect/disconnect"))
		return
	}

	// Connect (or re-connect) with the new configuration.

	var newToken, newEnv, newProg string
	if tok, ok := req.Form["token"]; ok {
		newToken = tok[0]
	} else {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("invalid POST: missing token"))
		return
	}
	if env, ok := req.Form["env"]; ok {
		newEnv = env[0]
	} else {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("invalid POST: missing env"))
		return
	}
	if prog, ok := req.Form["programName"]; ok {
		newProg = prog[0]
	} else {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("invalid POST: missing program name"))
		return
	}

	// Update the config stored in the HTTP handler.
	h.config.TenantToken = newToken
	h.config.ProgramName = newProg
	h.config.Environment = newEnv

	if newToken == "" {
		h.handleGet(w, "An API token is required.")
		return
	}
	if newProg == "" {
		h.handleGet(w, "A program name is required.")
		return
	}

	// If we're configured with a token, start a new connection to Side-Eye. If
	// there was a prior connection to Side-Eye, close it.
	Stop()

	err := Init(context.Background(), newProg, WithToken(newToken), WithEnvironment(newEnv))
	if err != nil {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("failed to update config: %w", err))
	}
	// Wait a little bit for the connection to be established before rendering
	// the page.
	timeout := time.Now().Add(time.Second)
	for {
		if time.Now().After(timeout) {
			break
		}
		if singletonConn.Status() != sideeyeconn.Connecting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Generate the page after the update.
	h.handleGet(w, "")
}

// handleGet handles a GET (as opposed to POST) request for generating the
// configuration page.
func (h *httpHandler) handleGet(w http.ResponseWriter, errMsg string) {
	w.WriteHeader(http.StatusOK)

	// Decide which config to render. If there is a connection, we use the
	// connection's config. Otherwise, we use the httpHandler's. Generally the two
	// are expected to agree, except in the case where the httpHandler was created
	// with different options.
	cfg := h.config
	s := singletonConn.Status()
	if s == sideeyeconn.Connected || s == sideeyeconn.Connecting {
		cfg = singletonConn.ActiveConfig
	}

	err := configPageTemplate.Execute(w, &templateData{
		Conn:    singletonConn,
		ErrMsg:  errMsg,
		Token:   cfg.TenantToken,
		Program: cfg.ProgramName,
		Env:     cfg.Environment,
	})
	if err != nil {
		singletonConn.ActiveConfig.ErrorLogger(fmt.Errorf("failed to write response: %w", err))
		return
	}
}

// templateData represents the data for the html template.
type templateData struct {
	Conn    *sideeyeconn.SideEyeConn
	ErrMsg  string
	Token   string
	Program string
	Env     string
}

func (td *templateData) ConnColor() string {
	switch td.Conn.Status() {
	case sideeyeconn.Uninitialized, sideeyeconn.Connecting, sideeyeconn.Disconnected:
		return "red"
	case sideeyeconn.Connected:
		return "green"
	default:
		return ""
	}
}

func (td *templateData) ConnStatus() string {
	switch td.Conn.Status() {
	case sideeyeconn.Uninitialized, sideeyeconn.Disconnected:
		return "disconnected"
	case sideeyeconn.Connected:
		return "connected"
	case sideeyeconn.Connecting:
		return "connecting"
	default:
		return ""
	}
}

func (td *templateData) DisconnectEnabled() bool {
	switch td.Conn.Status() {
	case sideeyeconn.Uninitialized, sideeyeconn.Disconnected:
		return false
	case sideeyeconn.Connected, sideeyeconn.Connecting:
		return true
	default:
		return false
	}
}
