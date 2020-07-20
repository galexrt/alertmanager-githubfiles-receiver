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

package template

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/models"
	"github.com/prometheus/alertmanager/notify/webhook"
	alert_template "github.com/prometheus/alertmanager/template"
)

// Templater takes care of templating our filenames and messages
type Templater struct {
	Repo  *models.Repo
	MSG   *webhook.Message
	Alert alert_template.Alert
	Time  time.Time
	Data  interface{}
}

// Time time info
type Time struct {
	Year int
}

// NewTemplater return a new Templater
func NewTemplater(r *models.Repo, msg *webhook.Message, alert alert_template.Alert) *Templater {
	return &Templater{
		Repo:  r,
		MSG:   msg,
		Alert: alert,
		Time:  time.Now(),
	}
}

// Template string
func (t *Templater) Template(in string) (string, error) {
	return t.TemplateWithData(in, nil)
}

// TemplateWithData string and add additional data
func (t *Templater) TemplateWithData(in string, data interface{}) (string, error) {
	tplFuncMap := make(template.FuncMap)
	tplFuncMap["Split"] = func(s string, d string) []string {
		return strings.Split(s, d)
	}

	tmpl, err := template.New("main").Funcs(tplFuncMap).Parse(in)
	if err != nil {
		return "", err
	}

	t.Data = data

	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, t); err != nil {
		return "", err
	}

	return tpl.String(), nil
}
