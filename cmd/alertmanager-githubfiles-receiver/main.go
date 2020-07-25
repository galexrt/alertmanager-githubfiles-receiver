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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/models"
	"github.com/galexrt/alertmanager-githubfiles-receiver/receiver"
	"github.com/google/go-github/v32/github"
	"github.com/mitchellh/go-homedir"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

var (
	cfgFile string

	receiverDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "githubfiles_receiver_duration_seconds",
			Help: "A histogram of request latencies to the receiver handler.",
		},
		[]string{"code"},
	)
	rootCmd = &cobra.Command{
		Use:   "alertmanager-githubfiles-receiver",
		Short: "Alertmanager GitHub files receiver allows for files to be created and update based on Prometheus alerts",
		RunE:  run,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cobra.yaml)")
	rootCmd.PersistentFlags().String("loglevel", "INFO", "logrus log level")
	rootCmd.PersistentFlags().String("listen", ":9959", "webhook receiver listen address")
	rootCmd.PersistentFlags().String("repo", "", "full name of repository (with owner)")
	rootCmd.PersistentFlags().String("dir", "content/issues", "base Path in the repository")
	rootCmd.PersistentFlags().String("filename", "", "fixed string or template filename to create at the dir in the repository.")
	rootCmd.PersistentFlags().String("content", "{{ . }}", "fixed string and / or template filename to create at the dir in the repository.")
	rootCmd.PersistentFlags().Duration("debouncedelay", 30*time.Second, "debounce delay")

	// Bind all flags
	viper.BindPFlags(rootCmd.PersistentFlags())
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}

		cwd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}

		// Search config in current working and home directory with name "githubfiles-receiver" (without extension).
		viper.AddConfigPath(cwd)
		viper.AddConfigPath(home)
		viper.SetConfigName("githubfiles-receiver")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		log.Infoln("Using config file:", viper.ConfigFileUsed())
	}

	// Set log level
	l, err := log.ParseLevel(viper.GetString("loglevel"))
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(l)
}

func main() {
	Execute()
}

// Execute run cobra command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	log.Info("starting alertmanager-githubfiles-receiver")

	targetSplit := strings.Split(viper.GetString("repo"), "/")
	if len(targetSplit) < 2 {
		log.Fatal("no or wrong format for target (repo) flag")
	}

	r := &models.Repo{
		Dir:   viper.GetString("dir"),
		Owner: targetSplit[0],
		Repo:  targetSplit[1],
	}

	ghToken := os.Getenv("GITHUB_TOKEN")

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: ghToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	filenameTmpl := viper.GetString("filename")
	handler := receiver.New(filenameTmpl, r, client)

	listenAddress := viper.GetString("listen")
	log.Infof("starting listener on %s", listenAddress)

	// TODO add signal handling
	stopCh := make(chan struct{})

	go handler.Run(stopCh)

	mux := http.NewServeMux()
	mux.Handle("/v1/receiver", promhttp.InstrumentHandlerDuration(receiverDuration, handler))
	return http.ListenAndServe(listenAddress, mux)
}
