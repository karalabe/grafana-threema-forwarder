// Copyright 2021 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	// Start the publisher goroutine to feed alerts to Threema
	alerts := make(chan *alert)
	go publisher(id, tos, alerts)

	// Create a forwarder REST service that accepts Grafana webhook POSTs,
	// converts them into Threema messages and relays them to the recipient.
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		// Retrieve the alert from the Grafana notification
		event := new(struct {
			State   string `json:"state"`
			Title   string `json:"title"`
			Message string `json:"message"`
			Image   string `json:"imageUrl"`
			Link    string `json:"ruleUrl"`
			Matches []struct {
				Metric string  `json:"metric"`
				Value  float64 `json:"value"`
			} `json:"evalMatches"`
		})
		if err := json.NewDecoder(req.Body).Decode(event); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// If an image was attached, try to download it
		var (
			image    []byte
			imageErr error
		)
		if len(event.Image) != 0 {
			res, err := http.Get(event.Image)
			if err != nil {
				imageErr = err
			} else {
				image, imageErr = ioutil.ReadAll(res.Body)
				res.Body.Close()
			}
		}
		// Prepare the alert message
		var icon string
		switch event.State {
		case "alerting":
			icon = "🔥"
			if strings.HasPrefix(event.Title, "[Alerting]") {
				event.Title = event.Title[10:]
			}
		case "ok":
			icon = "☘️"
			if strings.HasPrefix(event.Title, "[OK]") {
				event.Title = event.Title[4:]
			}
		default:
			icon = event.State
		}
		message := "*" + icon + " " + event.Title + "*\n\n"
		if imageErr != nil {
			message = message + "Failed to attach image: " + imageErr.Error() + "\n\n"
		}
		message = message + event.Message + "\n\n"

		for _, item := range event.Matches {
			message = message + fmt.Sprintf("*%s*: _%.2f_\n", item.Metric, item.Value)
		}
		if len(event.Matches) > 0 {
			message = message + "\n"
		}
		message = message + event.Link

		// Queue the message for Threema publishing
		alerts <- &alert{
			message: message,
			image:   image,
		}
	})
	http.ListenAndServe("0.0.0.0:8000", nil)
}

// alert is a helper struct to feed alerts over a channel to the publisher.
type alert struct {
	message string // Message content of the alert, always present
	image   []byte // Image content of the alert, optional
}

// publisher is an indefinite goroutine that keeps waiting for incoming alerts
// and publishes them over Threema. It's simpler to run a separate goroutine as
// it lower the number of reconnects in simultaneous alerts and also avoids the
// concurrency caused by the HTTP handler.
func publisher(id *threema.Identity, tos []string, alerts chan *alert) {
	for {
		// Wait for the next alert to arrive
		alert := <-alerts

		// Connect to the Threema network and send the alert message, looping
		// if a new one arrived in the meantime.
		log.Println("Connecting to the Threema network")
		conn, err := threema.Connect(id, new(threema.Handler)) // Ignore message
		if err != nil {
			log.Printf("Failed to connect to the Threema network: %v", err)
			continue // Alert lost - c'est la vie - maybe we'll succeed next time
		}
		for alert != nil {
			// Send the alert to all recipients
			for _, to := range tos {
				log.Printf("Sending alert message to %s", to)
				if len(alert.image) > 0 {
					if err := conn.SendImage(to, alert.image, alert.message); err != nil {
						log.Printf("Failed to send alert image: %v", err)
						continue // Alert lost - c'est la vie - maybe we'll succeed for the next user
					}
				} else {
					if err := conn.SendText(to, alert.message); err != nil {
						log.Printf("Failed to send alert message: %v", err)
						continue // Alert lost - c'est la vie - maybe we'll succeed for the next user
					}
				}
				log.Println("Alert message sent")
			}
			// Check if there are more alerts queued up
			select {
			case alert = <-alerts:
			default:
				alert = nil
			}
		}
		// All alerts queued up have been sent, disconnect
		conn.Close()
	}
}
