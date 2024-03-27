package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/pebble"
	jsoniter "github.com/json-iterator/go"
)

type Params struct {
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
	Kind   string `json:"kind"`
	Host   string `json:"host"`
	Key    string `json:"key"`
	Pak    string `json:"pak"`
	Waki   string `json:"waki"`
	NodeId string `json:"nodeid"`
	Rune   string `json:"rune"`

	Pin              string `json:"pin"`
	MinSendable      string `json:"minSendable"`
	MaxSendable      string `json:"maxSendable"`
	Npub             string `json:"npub"`
	NotifyZaps       bool   `json:"notifyzaps"`
	NotifyZapComment bool   `json:"notifycomments"`
	NotifyNonZap     bool   `json:"notifynonzaps"`
	Image            struct {
		DataURI string
		Bytes   []byte
		Ext     string
	}
}

func SaveName(
	name string,
	domain string,
	params *Params,
	providedPin string,
	overwrite bool,
	previousname string,
) (pin string, inv string, err error) {
	name = strings.ToLower(name)
	domain = strings.ToLower(domain)

	//remove trailing "/" from host
	host_formatted := params.Host
	params.Host = strings.TrimRight(host_formatted, "/")

	if params.Npub != "" && s.GetNostrProfile {
		NostrProfile, err := GetNostrProfileMetaData(params.Npub, 0)
		if err == nil {
			err = addImageToProfile(params, NostrProfile.Picture)
			if err != nil {

			}
		}

	}

	key := []byte(getID(name, domain))

	pin = ComputePIN(name, domain)

	if _, closer, err := db.Get(key); err == nil {
		defer closer.Close()
		if pin != providedPin {
			return "", "", errors.New("name already exists! must provide pin")
		}
	}
	if err != nil {
		return "", "", errors.New("that name does not exist")
	}

	if overwrite {
		previouskey := []byte(getID(previousname, domain))
		if err := db.Delete(previouskey, pebble.Sync); err != nil {
			return "", "", fmt.Errorf("couldn't delete previous entry: %w", err)
		}
	}

	params.Name = name
	params.Domain = domain

	if params.Kind != "forward" {
		// check if the given data works
		if inv, err = makeInvoice(params, 1000, &pin, "", ""); err != nil {
			return "", "", fmt.Errorf("couldn't make an invoice with the given data: %w", err)
		}

	}

	// save it
	data, _ := jsoniter.Marshal(params)
	if err := db.Set(key, data, pebble.Sync); err != nil {
		return "", "", err
	}

	return pin, inv, nil
}

func GetName(name, domain string) (*Params, error) {

	val, closer, err := db.Get([]byte(getID(name, domain)))
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var params Params
	if err := jsoniter.Unmarshal(val, &params); err != nil {
		return nil, err
	}

	params.Name = name
	params.Domain = domain
	return &params, nil
}

func GetAllUsers(domain string) ([]Params, error) {

	var k []byte
	var paramslist []Params

	iter, _ := db.NewIter(nil)
	for iter.SeekGE(k); iter.Valid(); iter.Next() {
		val, closer, err := db.Get([]byte(iter.Key()))
		if err != nil {
			return nil, err
		}
		defer closer.Close()

		var params Params
		if err := jsoniter.Unmarshal(val, &params); err != nil {
			return nil, err
		}
		params.Domain = domain
		paramslist = append(paramslist, params)

	}
	iter.Close()
	return paramslist, nil

}

func DeleteName(name, domain string) error {
	key := []byte(getID(name, domain))

	if err := db.Delete(key, pebble.Sync); err != nil {
		return err
	}

	return nil
}

func ComputePIN(name, domain string) string {
	mac := hmac.New(sha256.New, []byte(s.Secret))
	mac.Write([]byte(getID(name, domain)))
	return hex.EncodeToString(mac.Sum(nil))
}

func getID(name, domain string) string {
	if s.GlobalUsers {
		return strings.ToLower(name)
	} else {
		return strings.ToLower(fmt.Sprintf("%s@%s", name, domain))
	}
}

func tryMigrate(old, new string) {
	if _, err := os.Stat(old); os.IsNotExist(err) {
		return
	}

	log.Info().Str("db", old).Msg("Migrating db")

	newDb, err := pebble.Open(new, nil)
	if err != nil {
		log.Fatal().Err(err).Str("path", new).Msg("failed to open db.")
	}
	defer newDb.Close()

	oldDb, err := pebble.Open(old, nil)
	if err != nil {
		log.Fatal().Err(err).Str("path", old).Msg("failed to open db.")
	}
	defer oldDb.Close()

	iter, err := oldDb.NewIter(nil)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		log.Debug().Str("key", string(iter.Key())).Msg("Migrating key")
		var params Params
		if err := jsoniter.Unmarshal(iter.Value(), &params); err != nil {
			log.Debug().Err(err).Msg("Unmarshal error")
			continue
		}

		params.Domain = old // old database name was domain

		// save it
		data, err := jsoniter.Marshal(params)
		if err != nil {
			log.Debug().Err(err).Msg("Marshal error")
			continue
		}

		if err := newDb.Set([]byte(getID(params.Name, params.Domain)), data, pebble.Sync); err != nil {
			log.Debug().Err(err).Msg("Set error")
			continue
		}
	}
}
