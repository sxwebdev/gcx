package notify

import (
	"fmt"
	"log"

	"github.com/containrrr/shoutrrr"
	"github.com/sxwebdev/gcx/internal/tmpl"
)

// AlertData contains data for the notification message.
type AlertData struct {
	AppName string
	Version string
	Status  string
	Error   string
}

const alertTemplate = `Deployment Status Update
Application: {{.AppName}}
Version: {{.Version}}
Status: {{.Status}}
{{if .Error}}Error: {{.Error}}{{end}}`

// Send sends a notification through shoutrrr to the given URLs.
func Send(urls []string, data AlertData) error {
	if len(urls) == 0 {
		return nil
	}

	msg, err := tmpl.Process("alert", alertTemplate, data)
	if err != nil {
		return fmt.Errorf("process alert template: %w", err)
	}

	sender, err := shoutrrr.CreateSender(urls...)
	if err != nil {
		return fmt.Errorf("create alert sender: %w", err)
	}

	errs := sender.Send(msg, nil)
	var sendErrors []error
	for _, e := range errs {
		if e != nil {
			sendErrors = append(sendErrors, e)
			log.Printf("Failed to send alert: %v", e)
		}
	}

	if len(sendErrors) > 0 {
		return fmt.Errorf("failed to send %d alert(s)", len(sendErrors))
	}

	return nil
}
