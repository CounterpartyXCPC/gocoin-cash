// ======================================================================

//      cccccccccc          pppppppppp
//    cccccccccccccc      pppppppppppppp
//  ccccccccccccccc    ppppppppppppppppppp
// cccccc       cc    ppppppp        pppppp
// cccccc          pppppppp          pppppp
// cccccc        ccccpppp            pppppp
// cccccccc    cccccccc    pppp    ppppppp
//  ccccccccccccccccc     ppppppppppppppp
//     cccccccccccc      pppppppppppppp
//       cccccccc        pppppppppppp
//                       pppppp
//                       pppppp

// ======================================================================
// Copyright © 2018. Counterparty Cash Association (CCA) Zug, CH.
// All Rights Reserved. All work owned by CCA is herby released
// under Creative Commons Zero (0) License.

// Some rights of 3rd party, derivative and included works remain the
// property of thier respective owners. All marks, brands and logos of
// member groups remain the exclusive property of their owners and no
// right or endorsement is conferred by reference to thier organization
// or brand(s) by CCA.

// File:		sendtx.go
// Description:	Bictoin Cash webui Package

// Credits:

// Piotr Narewski, Gocoin Founder

// Julian Smith, Direction + Development
// Arsen Yeremin, Development
// Sumanth Kumar, Development
// Clayton Wong, Development
// Liming Jiang, Development

// Includes reference work of btsuite:

// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2018 The bcext developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Credits:

// Piotr Narewski, Gocoin Founder

// Julian Smith, Direction + Development
// Arsen Yeremin, Development
// Sumanth Kumar, Development
// Clayton Wong, Development
// Liming Jiang, Development

// Includes reference work of btsuite:

// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2018 The bcext developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Includes reference work of Bitcoin Core (https://github.com/bitcoin/bitcoin)
// Includes reference work of Bitcoin-ABC (https://github.com/Bitcoin-ABC/bitcoin-abc)
// Includes reference work of Bitcoin Unlimited (https://github.com/BitcoinUnlimited/BitcoinUnlimited/tree/BitcoinCash)
// Includes reference work of gcash by Shuai Qi "qshuai" (https://github.com/bcext/gcash)
// Includes reference work of gcash (https://github.com/gcash/bchd)

// + Other contributors

// =====================================================================

package webui

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/counterpartyxcpc/gocoin-cash/client/common"
	"github.com/counterpartyxcpc/gocoin-cash/client/usif"
	bch "github.com/counterpartyxcpc/gocoin-cash/lib/bch"
	"github.com/counterpartyxcpc/gocoin-cash/lib/bch_utxo"
)

const (
	AvgSignatureSize = 73
	AvgPublicKeySize = 34 /*Assumine compressed key*/
)

type MultisigAddr struct {
	MultiAddress               string
	ScriptPubKey               string
	KeysRequired, KeysProvided uint
	RedeemScript               string
	ListOfAddres               []string
}

func dl_payment(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.GetBool(&common.WalletON) {
		return
	}

	var err string

	if len(r.Form["outcnt"]) == 1 {
		var thisbal utxo.AllUnspentTx
		var pay_cmd string
		var totalinput, spentsofar uint64
		var change_addr *bch.BtcAddr

		tx := new(bch.Tx)
		tx.Version = 1
		tx.Lock_time = 0

		seq, er := strconv.ParseInt(r.Form["tx_seq"][0], 10, 64)
		if er != nil || seq < -2 || seq > 0xffffffff {
			err = "Incorrect Sequence value: " + r.Form["tx_seq"][0]
			goto error
		}

		outcnt, _ := strconv.ParseUint(r.Form["outcnt"][0], 10, 32)

		lck := new(usif.OneLock)
		lck.In.Add(1)
		lck.Out.Add(1)
		usif.LocksChan <- lck
		lck.In.Wait()
		defer lck.Out.Done()

		for i := 1; i <= int(outcnt); i++ {
			is := fmt.Sprint(i)
			if len(r.Form["txout"+is]) == 1 && r.Form["txout"+is][0] == "on" {
				hash := bch.NewUint256FromString(r.Form["txid"+is][0])
				if hash != nil {
					vout, er := strconv.ParseUint(r.Form["txvout"+is][0], 10, 32)
					if er == nil {
						var po = bch.TxPrevOut{Hash: hash.Hash, Vout: uint32(vout)}
						if res := common.BchBlockChain.Unspent.UnspentGet(&po); res != nil {
							addr := bch.NewAddrFromPkScript(res.Pk_script, common.Testnet)

							unsp := &utxo.OneUnspentTx{TxPrevOut: po, Value: res.Value,
								MinedAt: res.BchBlockHeight, Coinbase: res.WasCoinbase, BtcAddr: addr}

							thisbal = append(thisbal, unsp)

							// Add the input to our tx
							tin := new(bch.TxIn)
							tin.Input = po
							tin.Sequence = uint32(seq)
							tx.TxIn = append(tx.TxIn, tin)

							// Add the value to total input value
							totalinput += res.Value

							// If no change specified, use the first input addr as it
							if change_addr == nil {
								change_addr = addr
							}
						}
					}
				}
			}
		}

		if change_addr == nil {
			// There werte no inputs
			return
		}

		for i := 1; ; i++ {
			adridx := fmt.Sprint("adr", i)
			btcidx := fmt.Sprint("btc", i)

			if len(r.Form[adridx]) != 1 || len(r.Form[btcidx]) != 1 {
				break
			}

			if len(r.Form[adridx][0]) > 1 {
				addr, er := bch.NewAddrFromString(r.Form[adridx][0])
				if er == nil {
					am, er := bch.StringToSatoshis(r.Form[btcidx][0])
					if er == nil && am > 0 {
						if pay_cmd == "" {
							pay_cmd = "wallet -a=false -useallinputs -send "
						} else {
							pay_cmd += ","
						}
						pay_cmd += addr.String() + "=" + bch.UintToBtc(am)

						outs, er := bch.NewSpendOutputs(addr, am, common.CFG.Testnet)
						if er != nil {
							err = er.Error()
							goto error
						}
						tx.TxOut = append(tx.TxOut, outs...)

						spentsofar += am
					} else {
						err = "Incorrect amount (" + r.Form[btcidx][0] + ") for Output #" + fmt.Sprint(i)
						goto error
					}
				} else {
					err = "Incorrect address (" + r.Form[adridx][0] + ") for Output #" + fmt.Sprint(i)
					goto error
				}
			}
		}

		if pay_cmd == "" {
			err = "No inputs selected"
			goto error
		}

		pay_cmd += fmt.Sprint(" -seq ", seq)

		am, er := bch.StringToSatoshis(r.Form["txfee"][0])
		if er != nil {
			err = "Incorrect fee value: " + r.Form["txfee"][0]
			goto error
		}

		pay_cmd += " -fee " + r.Form["txfee"][0]
		spentsofar += am

		if len(r.Form["change"][0]) > 1 {
			addr, er := bch.NewAddrFromString(r.Form["change"][0])
			if er != nil {
				err = "Incorrect change address: " + r.Form["change"][0]
				goto error
			}
			change_addr = addr
		}
		pay_cmd += " -change " + change_addr.String()

		if totalinput > spentsofar {
			// Add change output
			outs, er := bch.NewSpendOutputs(change_addr, totalinput-spentsofar, common.CFG.Testnet)
			if er != nil {
				err = er.Error()
				goto error
			}
			tx.TxOut = append(tx.TxOut, outs...)
		}

		buf := new(bytes.Buffer)
		zi := zip.NewWriter(buf)

		was_tx := make(map[[32]byte]bool, len(thisbal))
		for i := range thisbal {
			if was_tx[thisbal[i].TxPrevOut.Hash] {
				continue
			}
			was_tx[thisbal[i].TxPrevOut.Hash] = true
			txid := bch.NewUint256(thisbal[i].TxPrevOut.Hash[:])
			fz, _ := zi.Create("balance/" + txid.String() + ".tx")
			if dat, er := common.GetRawTx(thisbal[i].MinedAt, txid); er == nil {
				fz.Write(dat)
			} else {
				println(er.Error())
			}
		}

		fz, _ := zi.Create("balance/unspent.txt")
		for i := range thisbal {
			fmt.Fprintln(fz, thisbal[i].UnspentTextLine())
		}

		if pay_cmd != "" {
			fz, _ = zi.Create(common.CFG.WebUI.PayCmdName)
			fz.Write([]byte(pay_cmd))
		}

		// Non-multisig transaction ...
		fz, _ = zi.Create("tx2sign.txt")
		fz.Write([]byte(hex.EncodeToString(tx.Serialize())))

		zi.Close()
		w.Header()["Content-Type"] = []string{"application/zip"}
		w.Write(buf.Bytes())
		return
	} else {
		err = "Bad request"
	}
error:
	s := load_template("send_error.html")
	write_html_head(w, r)
	s = strings.Replace(s, "<!--ERROR_MSG-->", err, 1)
	w.Write([]byte(s))
	write_html_tail(w)
}

func p_snd(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if !common.GetBool(&common.WalletON) {
		p_wallet_is_off(w, r)
		return
	}

	s := load_template("send.html")

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
