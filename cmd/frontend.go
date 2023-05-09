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
	FrontendViper *viper.Viper //nolint:gochecknoglobals // demo
)

// frontendCmd represents the frontend command
var frontendCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "frontend",
	Short: "Frontend",
	Long:  `Frontend server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())
		initConfigViper(FrontendViper)

		err := RunService(cmd.Context(), cmd.Use, args, &internal.FrontendConfig{}, FrontendViper, internal.NewFrontendService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(frontendCmd)
	frontendCmd.Flags().String("listenaddr", "localhost:8882", "Listen address")
	frontendCmd.Flags().String("instance", "#0", "Frontend instance")
	frontendCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector URL")
	frontendCmd.Flags().String("natsURL", "", "NATS URL")
	FrontendViper = viper.New()
	if err := FrontendViper.BindPFlags(frontendCmd.Flags()); err != nil {
		logger.GetLogger(frontendCmd.Use).Error(err, "Unable to bind flags")
		panic(err)
	}
}
