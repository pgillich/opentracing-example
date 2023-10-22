/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

var backendViper = viper.New() //nolint:gochecknoglobals // CMD

// backendCmd represents the backend command
var backendCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "backend",
	Short: "Backend",
	Long:  `Backend server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())

		err := RunService(cmd.Context(), cmd.Use, args, backendViper, &internal.BackendConfig{
			Command: fmt.Sprintf("%+v", cmd.Context().Value(model.CtxKeyCmd)),
		}, internal.NewBackendService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(backendCmd)
	backendCmd.Flags().String("listenaddr", "localhost:8881", "Listen address")
	backendCmd.Flags().String("instance", "#2", "Backend instance")
	backendCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector address")
	backendCmd.Flags().String("response", "Hello", "Response text")
	if err := backendViper.BindPFlags(backendCmd.Flags()); err != nil {
		logger.GetLogger(backendCmd.Use, slog.LevelDebug).Error("Unable to bind flags", logger.KeyError, err)
		panic(err)
	}
	backendViper.AutomaticEnv()
}
