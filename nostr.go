package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip19"
)

type Tag []string
type Tags []Tag
type NostrEvent struct {
	ID        string    `json:"id"`
	PubKey    string    `json:"pubkey"`
	CreatedAt time.Time `json:"created_at"`
	Kind      int       `json:"kind"`
	Tags      Tags      `json:"tags"`
	Content   string    `json:"content"`
	Sig       string    `json:"sig"`
}

var nip57Receipt nostr.Event
var zapEventSerializedStr string
var nip57ReceiptRelays []string

func Nip57DescriptionHash(zapEventSerialized string) string {
	hash := sha256.Sum256([]byte(zapEventSerialized))
	hashString := hex.EncodeToString(hash[:])
	return hashString
}

func DecodeBench32(key string) string {
	if _, v, err := nip19.Decode(key); err == nil {
		return v.(string)
	}
	return key

}

func EncodeBench32Public(key string) string {
	if v, err := nip19.EncodePublicKey(key); err == nil {
		return v
	}
	return key
}

func EncodeBench32Private(key string) string {
	if v, err := nip19.EncodePrivateKey(key); err == nil {
		return v
	}
	return key
}

func EncodeBench32Note(key string) string {
	if v, err := nip19.EncodeNote(key); err == nil {
		return v
	}
	return key
}

func sendMessage(receiverKey string, message string) {

	var relays []string
	var tags nostr.Tags
	reckey := DecodeBench32(receiverKey)
	tags = append(tags, nostr.Tag{"p", reckey})

	//references, err := optSlice(opts, "--reference")
	//if err != nil {
	//	return
	//}
	//for _, ref := range references {
	//tags = append(tags, nostr.Tag{"e", reckey})
	//}

	// parse and encrypt content
	privkeyhex := DecodeBench32(s.NostrPrivateKey)
	pubkey, _ := nostr.GetPublicKey(privkeyhex)

	sharedSecret, err := nip04.ComputeSharedSecret(reckey, privkeyhex)
	if err != nil {
		log.Printf("Error computing shared key: %s. x\n", err.Error())
		return
	}

	encryptedMessage, err := nip04.Encrypt(message, sharedSecret)
	if err != nil {
		log.Printf("Error encrypting message: %s. \n", err.Error())
		return
	}

	event := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: time.Now(),
		Kind:      nostr.KindEncryptedDirectMessage,
		Tags:      tags,
		Content:   encryptedMessage,
	}
	event.Sign(privkeyhex)
	publishNostrEvent(event, relays)
	log.Printf("%+v\n", event)
}

func handleNip05(w http.ResponseWriter, r *http.Request) {
	var err error
	var response string

	var allusers []Params
	allusers, err = GetAllUsers(s.Domain)
	firstpartstring := "{\n  \"names\": {\n"
	finalpartstring := " \t}\n}"
	var middlestring = ""

	for _, user := range allusers {
		nostrnpubHex := DecodeBench32(user.Npub)
		if user.Npub != "" { //do some more validation checks
			middlestring = middlestring + "\t\"" + user.Name + "\"" + ": " + "\"" + nostrnpubHex + "\"" + ",\n"
		}
	}

	if s.Nip05 {
		//Remove ',' from last entry
		if len(middlestring) > 2 {
			middlestringtrim := middlestring[:len(middlestring)-2]
			middlestringtrim += "\n"

			response = firstpartstring + middlestringtrim + finalpartstring
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintf(w, response)
	} else {
		return
	}

	if err != nil {
		return
	}
}

func GetNostrProfileMetaData(npub string) (nostr.ProfileMetadata, error) {
	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)

	var metadata *nostr.ProfileMetadata
	// connect to any relay
	url := "wss://relay.damus.io"
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		return *metadata, err
	}

	// create filters
	var filters nostr.Filters
	if _, v, err := nip19.Decode(npub); err == nil {
		t := make(map[string][]string)
		t["p"] = []string{v.(string)}
		filters = []nostr.Filter{{
			Authors: []string{v.(string)},
			Kinds:   []int{0},
			// limit = 3, get the three most recent notes
			Limit: 1,
		}}
	} else {
		return *metadata, err

	}
	sub, err := relay.Subscribe(ctx, filters)
	evs := make([]nostr.Event, 0)

	go func() {
		<-sub.EndOfStoredEvents

	}()

	for ev := range sub.Events {

		evs = append(evs, *ev)
	}
	relay.Close()

	if len(evs) > 0 {
		metadata, err = nostr.ParseMetadata(evs[0])
	} else {
		err = fmt.Errorf("no profile found for npub %s on relay %s", npub, url)
	}
	return *metadata, err

}

func publishNostrEvent(ev nostr.Event, relays []string) {

	// more relays
	relays = append(relays, "wss://relay.nostr.ch", "wss://eden.nostr.land", "wss://nostr.btcmp.com", "wss://nostr.relayer.se", "wss://relay.current.fyi", "wss://nos.lol", "wss://nostr.mom", "wss://relay.nostr.info", "wss://nostr.zebedee.cloud", "wss://nostr-pub.wellorder.net", "wss://relay.snort.social/", "wss://relay.damus.io/", "wss://nostr.oxtr.dev/", "wss://nostr.fmt.wiz.biz/", "wss://brb.io")
	// remove trailing /
	relays = cleanUrls(relays)
	// unique relays
	relays = uniqueSlice(relays)

	ev.Sign(s.NostrPrivateKey)

	//log.Printf("publishing nostr event %s", ev.ID)
	// publish the event to relays
	for _, url := range relays {
		go func(url string) {
			ctx := context.WithValue(context.Background(), "url", url)
			relay, e := nostr.RelayConnect(ctx, url)
			if e != nil {
				log.Error().Str(e.Error(), e.Error())
				return
			}
			time.Sleep(3 * time.Second)

			status, _ := relay.Publish(ctx, ev)
			log.Info().Str("[NOSTR] published to %s:", status.String())

			time.Sleep(3 * time.Second)
			relay.Close()
		}(url)

	}
}

func ExtractNostrRelays(zapEvent nostr.Event) []string {
	nip57ReceiptRelaysTags := zapEvent.Tags.GetFirst([]string{"relays"})
	if len(fmt.Sprintf("%s", nip57ReceiptRelaysTags)) > 0 {
		nip57ReceiptRelays = strings.Split(fmt.Sprintf("%s", nip57ReceiptRelaysTags), " ")
		// this dirty method returns slice [ "[relays", "wss...", "wss...", "wss...]" ] – we need to clean it up
		if len(nip57ReceiptRelays) > 1 {
			// remove the first entry
			nip57ReceiptRelays = nip57ReceiptRelays[1:]
			// clean up the last entry
			len_last_entry := len(nip57ReceiptRelays[len(nip57ReceiptRelays)-1])
			nip57ReceiptRelays[len(nip57ReceiptRelays)-1] = nip57ReceiptRelays[len(nip57ReceiptRelays)-1][:len_last_entry-1]
		}
	}
	return nip57ReceiptRelays
}

func CreateNostrReceipt(zapEvent nostr.Event, invoice string) nostr.Event {
	pk := nostrPrivkeyHex
	pub, _ := nostr.GetPublicKey(pk)
	zapEventSerialized, _ := json.Marshal(zapEvent)
	zapEventSerializedStr = fmt.Sprintf("%s", zapEventSerialized)
	nip57Receipt = nostr.Event{
		PubKey:    pub,
		CreatedAt: time.Now(),
		Kind:      9735,
		Tags: nostr.Tags{
			*zapEvent.Tags.GetFirst([]string{"p"}),
			[]string{"bolt11", invoice},
			[]string{"description", zapEventSerializedStr},
		},
	}
	if zapEvent.Tags.GetFirst([]string{"e"}) != nil {
		nip57Receipt.Tags = nip57Receipt.Tags.AppendUnique(*zapEvent.Tags.GetFirst([]string{"e"}))
	}
	nip57Receipt.Sign(pk)
	return nip57Receipt
}

func uniqueSlice(slice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func cleanUrls(slice []string) []string {
	list := []string{}
	for _, entry := range slice {
		if strings.HasSuffix(entry, "/") {
			entry = entry[:len(entry)-1]
		}
		list = append(list, entry)
	}
	return list
}
