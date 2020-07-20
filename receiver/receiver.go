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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/models"
	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/template"
	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/template/cstate"
	"github.com/google/go-github/v32/github"
	"github.com/prometheus/alertmanager/notify/webhook"
	alert_template "github.com/prometheus/alertmanager/template"
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

	r *models.Repo

	client *github.Client
}

// New return new Alerts Receiver
func New(filenameTmpl string, r *models.Repo, client *github.Client) *Receiver {
	return &Receiver{
		filenameTmpl: filenameTmpl,
		r:            r,
		client:       client,
	}
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
		log.Error(err)
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
		if err := r.handleAlert(msg, alert); err != nil {
			return err
		}
	}

	return nil
}

func (r *Receiver) handleAlert(msg *webhook.Message, alert alert_template.Alert) error {
	t := template.NewTemplater(r.r, msg, alert)

	fileName, err := t.Template(r.filenameTmpl)
	if err != nil {
		return err
	}
	fileName = strings.TrimSpace(fileName)
	fileNameSplit := strings.Split(fileName, "/")
	_ = fileNameSplit

	opts := &github.RepositoryContentGetOptions{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, _, _, err := r.client.Repositories.GetContents(ctx, r.r.Owner, r.r.Repo, path.Join(r.r.Dir, fileName), opts)
	if err != nil {
		return err
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

	needToCreateFile := false
	if fileContent == "" {
		needToCreateFile = true
	}

	switch viper.GetString("engine") {
	case "cstate":
		fileContent, err = cstate.Template(t, fileContent)
		if err != nil {
			return err
		}
	}

	fmt.Printf("content:\n%s\n", fileContent)

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
