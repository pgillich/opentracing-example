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

// frontendCmd represents the frontend command
var frontendCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "frontend",
	Short: "Frontend",
	Long:  `Frontend server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())

		err := RunService(cmd.Context(), cmd.Use, args, &internal.FrontendConfig{}, internal.NewFrontendService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(frontendCmd)
	frontendCmd.Flags().String("listenaddr", "localhost:8882", "Listen address")
	frontendCmd.Flags().String("instance", "#0", "Frontend instance")
	frontendCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector address")
	if err := viper.BindPFlags(frontendCmd.Flags()); err != nil {
		logger.GetLogger(frontendCmd.Use).Error(err, "Unable to bind flags")
		panic(err)
	}
}
