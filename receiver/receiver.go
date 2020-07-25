/*
Copyright 2019 Alexander Trost <galexrt@googlemail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package receiver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/models"
	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/template"
	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/template/cstate"
	"github.com/google/go-github/v32/github"
	"github.com/prometheus/alertmanager/notify/webhook"
	alert_template "github.com/prometheus/alertmanager/template"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	// EnabledRuleLabel label the receiver is looking out for if it should act
	EnabledRuleLabel = "githubfilesenabled"
)

// Receiver webhook receiver
type Receiver struct {
	filenameTmpl string

	ch             chan alert_template.Alert
	alerts         map[string]alert_template.Alert
	alertsMutex    sync.Mutex
	timeQueue      map[string]time.Time
	timeQueueMutex sync.Mutex

	r      *models.Repo
	client *github.Client
}

// New return new Alerts Receiver
func New(filenameTmpl string, r *models.Repo, client *github.Client) *Receiver {
	return &Receiver{
		filenameTmpl:   filenameTmpl,
		ch:             make(chan alert_template.Alert),
		alerts:         map[string]alert_template.Alert{},
		alertsMutex:    sync.Mutex{},
		timeQueue:      map[string]time.Time{},
		timeQueueMutex: sync.Mutex{},
		r:              r,
		client:         client,
	}
}

// Run
func (r *Receiver) Run(stopCh chan struct{}) error {
	go func() {
		select {
		case <-time.After(time.Second):
			r.timeQueueMutex.Lock()
			for alertHash, t := range r.timeQueue {
				if time.Now().After(t) {
					log.Debugf("alert %s in timequeue not yet after debounce delay", alertHash)
					continue
				}

				alert, ok := r.getAlertByHash(alertHash)
				if !ok {
					log.Warnf("didn't find alert for hash %s in alerts list, even though it is in the timeQueue", alertHash)
					continue
				}

				// Remove it from the timeQueue and the alerts list
				delete(r.timeQueue, alertHash)
				r.alertsMutex.Lock()
				delete(r.alerts, alertHash)
				r.alertsMutex.Unlock()

				// Handle alert now
				if err := r.handleAlert(alert); err != nil {
					log.Errorf("error while handling alert. %+v", err)
					continue
				}
			}
			r.timeQueueMutex.Unlock()
		case <-stopCh:
			return
		}
	}()

	debounceDelay := viper.GetDuration("debounceDelay")

	select {
	case alert := <-r.ch:
		alertHash := generateAlertHash(alert.Labels)
		func() {
			r.alertsMutex.Lock()
			defer r.alertsMutex.Unlock()

			item, ok := r.alerts[alertHash]
			if ok {
				alert = item
			} else {
				r.alerts[alertHash] = alert
			}

			r.timeQueueMutex.Lock()
			defer r.timeQueueMutex.Unlock()

			r.timeQueue[alertHash] = time.Now().Add(debounceDelay)
		}()
	case <-stopCh:
		return nil
	}
	return nil
}

func (r *Receiver) getAlertByHash(hash string) (alert_template.Alert, bool) {
	r.alertsMutex.Lock()
	defer r.alertsMutex.Unlock()
	alert, ok := r.alerts[hash]
	return alert, ok
}

// generateAlertHash generate sha256 sum from the alert labels
func generateAlertHash(labels alert_template.KV) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", labels)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

func (r *Receiver) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	log.Info("request received")

	if req.Method != http.MethodPost {
		res.WriteHeader(http.StatusBadRequest)
		res.Write([]byte("Only POST requests are allowed"))
		return
	}

	// Read request body.
	alertBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Errorf("failed to read request body: %s", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// The WebhookMessage is dependent on alertmanager version. Parse it.
	msg := &webhook.Message{}
	if err := json.Unmarshal(alertBytes, msg); err != nil {
		log.Errorf("failed to parse webhook message from %s: %s", req.RemoteAddr, err)
		log.Debugf("webhook message %s", string(alertBytes))
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := r.handleWebhook(msg); err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (r *Receiver) handleWebhook(msg *webhook.Message) error {
	for _, alert := range msg.Alerts {
		requiredLabelsSet := false
		for k := range alert.Labels {
			if k == EnabledRuleLabel {
				requiredLabelsSet = true
			}
		}
		if !requiredLabelsSet {
			log.Error("the required label is not set")
			continue
		}

		name, ok := alert.Labels[model.AlertNameLabel]
		if !ok {
			name = "N/A"
		}
		log.Infof("handling webhook for alert %s", name)
		r.ch <- alert
	}

	return nil
}

func (r *Receiver) handleAlert(alert alert_template.Alert) error {
	t := template.NewTemplater(r.r, alert)

	fileName, err := t.Template(r.filenameTmpl)
	if err != nil {
		return err
	}
	fileName = strings.TrimSpace(fileName)

	needToCreateFile := false

	opts := &github.RepositoryContentGetOptions{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, _, resp, err := r.client.Repositories.GetContents(ctx, r.r.Owner, r.r.Repo, path.Join(r.r.Dir, fileName), opts)
	if resp.StatusCode == http.StatusNotFound {
		needToCreateFile = true
	} else {
		if err != nil {
			return err
		}
	}

	fileContent := ""
	var sha *string
	if file != nil {
		sha = file.SHA
		fileContent, err = file.GetContent()
		if err != nil {
			return err
		}
	}

	switch viper.GetString("engine") {
	case "cstate":
		fileContent, err = cstate.Template(t, fileContent)
		if err != nil {
			return err
		}
	}

	// If log level is debug or trace show message content
	if log.GetLevel() == log.DebugLevel || log.GetLevel() == log.TraceLevel {
		fmt.Printf("content:\n%s\n", fileContent)
	}

	if err := r.createOrUpdateFileInRepo(path.Join(r.r.Dir, fileName), fileContent, needToCreateFile, sha); err != nil {
		return err
	}

	return nil
}

func (r *Receiver) createOrUpdateFileInRepo(filename string, content string, needToCreateFile bool, sha *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &github.RepositoryContentFileOptions{
		Message:   github.String(viper.GetString("commitMessage")),
		Content:   []byte(content),
		Branch:    github.String(viper.GetString("branch")),
		Committer: &github.CommitAuthor{Name: github.String(viper.GetString("commitName")), Email: github.String(viper.GetString("commitEmail"))},
		SHA:       sha,
	}

	if needToCreateFile {
		_, _, err := r.client.Repositories.CreateFile(ctx, r.r.Owner, r.r.Repo, filename, opts)
		return err
	}
	_, _, err := r.client.Repositories.UpdateFile(ctx, r.r.Owner, r.r.Repo, filename, opts)
	return err
}
