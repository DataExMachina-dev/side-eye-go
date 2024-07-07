package sideeye

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func HttpHandler() http.Handler {
	return httpHandler{}
}

type httpHandler struct{}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// GETs renders the current state of the connection.
	if req.Method == http.MethodGet {
		h.handleGet(w)
		return
	}

	// If this is a POST request, stop the old connection (if any) and, if a token
	// is specified, start a new connection using it.
	if err := req.ParseForm(); err != nil {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("failed to parse form: %w", err))
		return
	}

	if _, ok := req.Form["disconnect"]; ok {
		Stop()
		h.handleGet(w)
		return
	}

	if _, ok := req.Form["connect"]; !ok {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("invalid POST: missing connect/disconnect"))
		return
	}

	// Connect (or re-connect) with the new configuration.

	var newToken, newEnv string
	if tok, ok := req.Form["token"]; ok {
		newToken = tok[0]
	} else {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("invalid POST: missing token"))
		return
	}
	if env, ok := req.Form["env"]; ok {
		newEnv = env[0]
	} else {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("invalid POST: missing env"))
		return
	}

	// If there was a prior connection to Side-Eye, close it.
	Stop()

	// If we're configured with a token, start a new connection to Side-Eye.
	if newToken != "" {
		err := Init(context.Background(), WithToken(newToken), WithEnvironment(newEnv))
		if err != nil {
			singletonConn.activeConfig.errorLogger(fmt.Errorf("failed to update config: %w", err))
		}
		// Wait a little bit for the connection to be established before rendering
		// the page.
		timeout := time.Now().Add(time.Second)
		for {
			if time.Now().After(timeout) {
				break
			}
			if singletonConn.Status() != Connecting {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Generate the page after the update.
	h.handleGet(w)
}

func (h httpHandler) handleGet(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)

	status := singletonConn.Status()
	var statusStr, color string
	switch status {
	case Connected:
		statusStr = "connected"
		color = "green"
	case Disconnected:
		statusStr = "disconnected"
		color = "red"
	case Connecting:
		statusStr = "connecting"
		color = "red"
	}

	sb := strings.Builder{}
	sb.WriteString(`<html>
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
<form action="" method="POST">
<div style="
	display:grid;
	gap:3px;
	grid-template-columns: 9em 20em;
	margin-bottom: 10px;"
	>
`)
	sb.WriteString(fmt.Sprintf(`
<div>Connection status:</div>
<div style="display:flex; flex-direction:row; align-items:center; gap:3px">
	<div class="circle" style="background-color:%s;"></div>
	<span>%s</span>
</div>`, color, statusStr))
	sb.WriteString("<div>Side-Eye token:</div>")
	sb.WriteString(fmt.Sprintf(`<input type="text" name="token" value="%s"/>`,
		singletonConn.activeConfig.tenantToken))
	sb.WriteString("<div>Environment:</div>")
	sb.WriteString(fmt.Sprintf(`<input type="text" name="env" value="%s"/>`,
		singletonConn.activeConfig.environment))

	disconnectAttribute := ""
	if status == Disconnected {
		disconnectAttribute = "disabled"
	}

	sb.WriteString(fmt.Sprintf(`
</div>
<input type="submit" value="Reconnect" name="connect"/>
<input type="submit" value="Disconnect" name="disconnect" %s/>
</form>
</body>
</html>`, disconnectAttribute))

	_, err := w.Write([]byte(sb.String()))
	if err != nil {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("failed to write response: %w", err))
	}
	return
}
