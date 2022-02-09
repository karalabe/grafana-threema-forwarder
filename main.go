// Copyright 2021 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/karalabe/go-threema"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	identityFlag        string
	passwordFlag        string
	recipientIDFlag     string
	recipientPubKeyFlag string
)

func main() {
	viper.AutomaticEnv()

	rootCmd := &cobra.Command{
		Use:   "grafana-threema-forwarder",
		Short: "Grafana to Threema alert forwarder",
		Run:   forwarder,
	}
	rootCmd.Flags().StringVar(&identityFlag, "id", viper.GetString("G2T_ID_BACKUP"), "Exported and password protected Threema identity (G2T_ID_BACKUP)")
	rootCmd.Flags().StringVar(&passwordFlag, "id.secret", viper.GetString("G2T_ID_SECRET"), "Decryption password used to export the identity (G2T_ID_SECRET)")
	rootCmd.Flags().StringVar(&recipientIDFlag, "to", viper.GetString("G2T_RCPT_ID"), "Threema ID(s) to forward the Grafana alerts to (G2T_RCPT_ID)")
	rootCmd.Flags().StringVar(&recipientPubKeyFlag, "to.pubkey", viper.GetString("G2T_RCPT_PUBKEY"), "Threema public key(s) of the recipient(s) (G2T_RCPT_PUBKEY)")

	rootCmd.Execute()
}

func forwarder(cmd *cobra.Command, args []string) {
	// Construct the sender identity with the recipient as a contact
	log.Println("Loading local and remote identity")
	id, err := threema.Identify(identityFlag, passwordFlag)
	if err != nil {
		log.Fatalf("Failed to load sender identity: %v", err)
	}
	var (
		tos  = strings.Split(recipientIDFlag, ",")
		keys = strings.Split(recipientPubKeyFlag, ",")
	)
	if len(tos) == 0 {
		log.Fatalf("No recpient IDs provided")
	}
	if len(tos) != len(keys) {
		log.Fatalf("Mismatchine recipient IDs and pubkeys: %d ids, %d pubkeys", len(tos), len(keys))
	}
	for i, to := range tos {
		if err := id.Trust(to, keys[i]); err != nil {
			log.Fatalf("Failed to add recipient %d as contact: %v", i, err)
		}
	}
	// Create a forwarder REST service that accepts Grafana webhook POSTs,
	// converts them into Threema messages and relays them to the recipient.
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		// Retrieve the alert from the Grafana notification
		alert := new(struct {
			State   string `json:"state"`
			Title   string `json:"title"`
			Message string `json:"message"`
			Image   string `json:"imageUrl"`
		})
		if err := json.NewDecoder(req.Body).Decode(alert); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// If an image was attached, try to download it

		// Connect to the Threema network and send the alert message
		log.Println("Connecting to the Threema network")
		conn, err := threema.Connect(id, new(threema.Handler)) // Ignore message
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to connect to the Threema network: %v", err), http.StatusInternalServerError)
			return
		}
		log.Println("Sending alert message")
		var icon string
		switch alert.State {
		case "alerting":
			icon = "⚠"
		case "ok":
			icon = "💚"
		default:
			icon = alert.State
		}
		if err := conn.SendText(recipientIDFlag, icon+"️ "+alert.Title); err != nil {
			http.Error(w, fmt.Sprintf("Failed to send alert title: %v", err), http.StatusInternalServerError)
			return
		}
		if err := conn.SendText(recipientIDFlag, alert.Message); err != nil {
			http.Error(w, fmt.Sprintf("Failed to send alert message: %v", err), http.StatusInternalServerError)
			return
		}
		log.Println("Alert message sent")
		conn.Close()
	})
	http.ListenAndServe("0.0.0.0:8000", nil)
}