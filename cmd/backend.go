/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
)

var (
	BackendViper   *viper.Viper //nolint:gochecknoglobals // demo
	BackendNatsURL *string      //nolint:gochecknoglobals // demo
)

// backendCmd represents the backend command
var backendCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "backend",
	Short: "Backend",
	Long:  `Backend server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())
		initConfigViper(BackendViper)
		cfg := &internal.BackendConfig{
			NatsURL: *BackendNatsURL,
		}
		err := RunService(cmd.Context(), cmd.Use, args, cfg, BackendViper, internal.NewBackendService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(backendCmd)
	backendCmd.Flags().String("listenaddr", "localhost:8881", "Listen address")
	backendCmd.Flags().String("instance", "#2", "Backend instance")
	backendCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector URL")
	BackendNatsURL = backendCmd.Flags().String("natsURL", "", "NATS URL")
	backendCmd.Flags().String("response", "Hello", "Response text")
	BackendViper = viper.New()

	if err := BackendViper.BindPFlags(backendCmd.Flags()); err != nil {
		logger.GetLogger(backendCmd.Use).Error(err, "Unable to bind flags")
		panic(err)
	}
}
