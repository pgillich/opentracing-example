/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"time"

	"github.com/sagikazarmark/slog-shim"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

var oidcViper = viper.New() //nolint:gochecknoglobals // CMD

// oidcCmd represents the oidc command
var oidcCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "oidc",
	Short: "Oidc",
	Long:  `Oidc command`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())

		err := RunService(cmd.Context(), cmd.Use, args, oidcViper, &internal.OidcConfig{
			Command: fmt.Sprintf("%+v", cmd.Context().Value(model.CtxKeyCmd)),
		}, internal.NewOidcService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(oidcCmd)
	oidcCmd.Flags().String("listenaddr", "localhost:5556", "Listen address")
	oidcCmd.Flags().String("instance", "#O", "Oidc instance")
	oidcCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector address")
	oidcCmd.Flags().String("oauth2ClientID", "", "Oauth2 client ID")
	oidcCmd.Flags().String("oauth2ClientSecret", "", "Oauth2 client secret")
	oidcCmd.Flags().Int("httpClientCaptureMode", 0, "HTTP Client capture mode. 0: none, 1: capture, 2: fake")
	oidcCmd.Flags().String("httpClientCaptureDir", "", "HTTP Client capture dir")
	oidcCmd.Flags().String("oidcProviderURL", "https://accounts.google.com", "OIDC provider URL")
	oidcCmd.Flags().String("oidcRedirectPath", "/auth/google/callback", "OIDC redirect URL")

	if err := oidcViper.BindPFlags(oidcCmd.Flags()); err != nil {
		logger.GetLogger(oidcCmd.Use, slog.LevelDebug).Error("Unable to bind flags", logger.KeyError, err)
		panic(err)
	}
	oidcViper.AutomaticEnv()
}
