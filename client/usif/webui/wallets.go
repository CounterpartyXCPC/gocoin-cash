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

// File:		wallets.go
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
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/counterpartyxcpc/gocoin-cash/client/common"
	"github.com/counterpartyxcpc/gocoin-cash/client/network"
	"github.com/counterpartyxcpc/gocoin-cash/client/usif"
	"github.com/counterpartyxcpc/gocoin-cash/client/wallet"
	bch "github.com/counterpartyxcpc/gocoin-cash/lib/bch"
	"github.com/counterpartyxcpc/gocoin-cash/lib/bch_utxo"
)

func p_wal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if !common.GetBool(&common.WalletON) {
		p_wallet_is_off(w, r)
		return
	}

	var str string
	common.Last.Mutex.Lock()
	if common.BchBlockChain.Consensus.Enforce_SEGWIT != 0 &&
		common.Last.BchBlock.Height >= common.BchBlockChain.Consensus.Enforce_SEGWIT {
		str = "var segwit_active=true"
	} else {
		str = "var segwit_active=false"
	}
	common.Last.Mutex.Unlock()
	page := load_template("wallet.html")
	page = strings.Replace(page, "/*WALLET_JS_VARS*/", str, 1)
	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}

func getaddrtype(aa *bch.BtcAddr) string {
	if aa.SegwitProg != nil && aa.SegwitProg.Version == 0 && len(aa.SegwitProg.Program) == 20 {
		return "P2WPKH"
	}
	if aa.Version == bch.AddrVerPubkey(common.Testnet) {
		return "P2PKH"
	}
	if aa.Version == bch.AddrVerScript(common.Testnet) {
		return "P2SH"
	}
	return "unknown"
}

func json_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.GetBool(&common.WalletON) {
		return
	}

	if r.Method != "POST" {
		return
	}

	summary := len(r.Form["summary"]) > 0
	mempool := len(r.Form["mempool"]) > 0
	getrawtx := len(r.Form["rawtx"]) > 0

	inp, er := ioutil.ReadAll(r.Body)
	if er != nil {
		println(er.Error())
		return
	}

	var addrs []string
	er = json.Unmarshal(inp, &addrs)
	if er != nil {
		println(er.Error())
		return
	}

	type OneOut struct {
		TxId     string
		Vout     uint32
		Value    uint64
		Height   uint32
		Coinbase bool
		Message  string
		Addr     string
		AddrType string
		Spending bool   // if true the spending tx is in the mempool
		RawTx    string `json:",omitempty"`
	}

	type OneOuts struct {
		Value            uint64
		OutCnt           int
		SegWitCnt        int
		SegWitAddr       string
		SegWitNativeCnt  int
		SegWitNativeAddr string
		Outs             []OneOut

		PendingCnt   int
		PendingValue uint64
		PendingOuts  []OneOut

		SpendingValue uint64
		SpendingCnt   uint64
	}

	out := make(map[string]*OneOuts)

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	var addr_map map[string]string

	if mempool {
		// make addrs -> idx
		addr_map = make(map[string]string, 2*len(addrs))
	}

	for _, a := range addrs {
		aa, e := bch.NewAddrFromString(a)
		if e != nil {
			continue
		}

		unsp := wallet.GetAllUnspent(aa)
		newrec := new(OneOuts)
		if len(unsp) > 0 {
			newrec.OutCnt = len(unsp)
			for _, u := range unsp {
				newrec.Value += u.Value
				network.TxMutex.Lock()
				_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
				network.TxMutex.Unlock()
				if spending {
					newrec.SpendingValue += u.Value
					newrec.SpendingCnt++
				}
				if !summary {
					txid := bch.NewUint256(u.TxPrevOut.Hash[:])
					var rawtx string
					if getrawtx {
						dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
						if er == nil {
							rawtx = hex.EncodeToString(dat)
						}
					}
					newrec.Outs = append(newrec.Outs, OneOut{
						TxId: bch.NewUint256(u.TxPrevOut.Hash[:]).String(), Vout: u.Vout,
						Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
						Message: html.EscapeString(string(u.Message)), Addr: a, Spending: spending,
						RawTx: rawtx, AddrType: getaddrtype(aa)})
				}
			}
		}

		out[a] = newrec

		if mempool {
			addr_map[string(aa.OutScript())] = a
		}

		/* For P2KH addr, we wlso check its segwit's P2SH-P2WPKH and Native P2WPKH */
		if aa.SegwitProg == nil && aa.Version == bch.AddrVerPubkey(common.Testnet) {
			p2kh := aa.Hash160

			// P2SH SegWit if applicable
			h160 := bch.Rimp160AfterSha256(append([]byte{0, 20}, p2kh[:]...))
			aa = bch.NewAddrFromHash160(h160[:], bch.AddrVerScript(common.Testnet))
			newrec.SegWitAddr = aa.String()
			unsp = wallet.GetAllUnspent(aa)
			if len(unsp) > 0 {
				newrec.OutCnt += len(unsp)
				newrec.SegWitCnt = len(unsp)
				as := aa.String()
				for _, u := range unsp {
					newrec.Value += u.Value
					network.TxMutex.Lock()
					_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
					network.TxMutex.Unlock()
					if spending {
						newrec.SpendingValue += u.Value
						newrec.SpendingCnt++
					}
					if !summary {
						txid := bch.NewUint256(u.TxPrevOut.Hash[:])
						var rawtx string
						if getrawtx {
							dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
							if er == nil {
								rawtx = hex.EncodeToString(dat)
							}
						}
						newrec.Outs = append(newrec.Outs, OneOut{
							TxId: txid.String(), Vout: u.Vout,
							Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
							Message: html.EscapeString(string(u.Message)), Addr: as,
							Spending: spending, RawTx: rawtx, AddrType: "P2SH-P2WPKH"})
					}
				}
			}
			if mempool {
				addr_map[string(aa.OutScript())] = a
			}

			// Native SegWit if applicable
			aa = bch.NewAddrFromPkScript(append([]byte{0, 20}, p2kh[:]...), common.Testnet)
			newrec.SegWitNativeAddr = aa.String()
			unsp = wallet.GetAllUnspent(aa)
			if len(unsp) > 0 {
				newrec.OutCnt += len(unsp)
				newrec.SegWitNativeCnt = len(unsp)
				as := aa.String()
				for _, u := range unsp {
					newrec.Value += u.Value
					network.TxMutex.Lock()
					_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
					network.TxMutex.Unlock()
					if spending {
						newrec.SpendingValue += u.Value
						newrec.SpendingCnt++
					}
					if !summary {
						txid := bch.NewUint256(u.TxPrevOut.Hash[:])
						var rawtx string
						if getrawtx {
							dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
							if er == nil {
								rawtx = hex.EncodeToString(dat)
							}
						}
						newrec.Outs = append(newrec.Outs, OneOut{
							TxId: txid.String(), Vout: u.Vout,
							Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
							Message: html.EscapeString(string(u.Message)), Addr: as,
							Spending: spending, RawTx: rawtx, AddrType: "P2WPKH"})
					}
				}
			}
			if mempool {
				addr_map[string(aa.OutScript())] = a
			}

		}
	}

	// check memory pool
	if mempool {
		network.TxMutex.Lock()
		for _, t2s := range network.TransactionsToSend {
			for vo, to := range t2s.TxOut {
				if a, ok := addr_map[string(to.Pk_script)]; ok {
					newrec := out[a]
					newrec.PendingValue += to.Value
					newrec.PendingCnt++
					if !summary {
						po := &bch.TxPrevOut{Hash: t2s.Hash.Hash, Vout: uint32(vo)}
						_, spending := network.SpentOutputs[po.UIdx()]
						newrec.PendingOuts = append(newrec.PendingOuts, OneOut{
							TxId: t2s.Hash.String(), Vout: uint32(vo),
							Value: to.Value, Spending: spending})
					}
				}
			}
		}
		network.TxMutex.Unlock()
	}

	lck.Out.Done()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func dl_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.GetBool(&common.WalletON) {
		return
	}

	if r.Method != "POST" {
		return
	}

	var addrs []string
	var labels []string

	if len(r.Form["addrcnt"]) != 1 {
		println("no addrcnt")
		return
	}
	addrcnt, _ := strconv.ParseUint(r.Form["addrcnt"][0], 10, 32)

	for i := 0; i < int(addrcnt); i++ {
		is := fmt.Sprint(i)
		if len(r.Form["addr"+is]) == 1 {
			addrs = append(addrs, r.Form["addr"+is][0])
			if len(r.Form["label"+is]) == 1 {
				labels = append(labels, r.Form["label"+is][0])
			} else {
				labels = append(labels, "")
			}
		}
	}

	type one_unsp_rec struct {
		bch.TxPrevOut
		Value    uint64
		Addr     string
		MinedAt  uint32
		Coinbase bool
	}

	var thisbal utxo.AllUnspentTx

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	for idx, a := range addrs {
		aa, e := bch.NewAddrFromString(a)
		aa.Extra.Label = labels[idx]
		if e == nil {
			newrecs := wallet.GetAllUnspent(aa)
			if len(newrecs) > 0 {
				thisbal = append(thisbal, newrecs...)
			}

			/* Segwit P2WPKH: */
			if aa.SegwitProg == nil && aa.Version == bch.AddrVerPubkey(common.Testnet) {
				p2kh := aa.Hash160

				// P2SH SegWit if applicable
				h160 := bch.Rimp160AfterSha256(append([]byte{0, 20}, aa.Hash160[:]...))
				aa = bch.NewAddrFromHash160(h160[:], bch.AddrVerScript(common.Testnet))
				newrecs = wallet.GetAllUnspent(aa)
				if len(newrecs) > 0 {
					thisbal = append(thisbal, newrecs...)
				}

				// Native SegWit if applicable
				aa = bch.NewAddrFromPkScript(append([]byte{0, 20}, p2kh[:]...), common.Testnet)
				newrecs = wallet.GetAllUnspent(aa)
				if len(newrecs) > 0 {
					thisbal = append(thisbal, newrecs...)
				}
			}
		}
	}
	lck.Out.Done()

	buf := new(bytes.Buffer)
	zi := zip.NewWriter(buf)
	was_tx := make(map[[32]byte]bool)

	sort.Sort(thisbal)
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

	zi.Close()
	w.Header()["Content-Type"] = []string{"application/zip"}
	w.Write(buf.Bytes())

}

func json_wallet_status(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		WalletON       bool
		WalletProgress uint32
		WalletOnIn     uint32
	}
	common.LockCfg()
	out.WalletON = common.WalletON
	out.WalletProgress = common.WalletProgress
	out.WalletOnIn = common.WalletOnIn
	common.UnlockCfg()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
