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

// File:		unspent_db.go
// Description:	Bictoin Cash utxo Package

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

package utxo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"time"

	bch "github.com/counterpartyxcpc/gocoin-cash/lib/bch"
	"github.com/counterpartyxcpc/gocoin-cash/lib/others/sys"
)

const (
	UTXO_RECORDS_PREALLOC = 25e6
)

var (
	UTXO_WRITING_TIME_TARGET        = 5 * time.Minute // Take it easy with flushing UTXO.db onto disk
	UTXO_SKIP_SAVE_BLOCKS    uint32 = 0
)

type FunctionWalkUnspent func(*UtxoRec)

type CallbackFunctions struct {
	// If NotifyTx is set, it will be called each time a new unspent
	// output is being added or removed. When being removed, bch.TxOut is nil.
	NotifyTxAdd func(*UtxoRec)
	NotifyTxDel func(*UtxoRec, []bool)
}

// Used to pass block's changes to UnspentDB
type BchBlockChanges struct {
	Height          uint32
	LastKnownHeight uint32 // put here zero to disable this feature
	AddList         []*UtxoRec
	DeledTxs        map[[32]byte][]bool
	UndoData        map[[32]byte]*UtxoRec
}

type UnspentDB struct {
	HashMap      map[UtxoKeyType][]byte
	sync.RWMutex // used to access HashMap

	LastBlockHash      []byte
	LastBlockHeight    uint32
	dir_utxo, dir_undo string
	volatimemode       bool
	UnwindBufLen       uint32
	DirtyDB            sys.SyncBool
	sync.Mutex

	abortwritingnow   chan bool
	WritingInProgress sys.SyncBool
	writingDone       sync.WaitGroup
	lastFileClosed    sync.WaitGroup

	CurrentHeightOnDisk uint32
	hurryup             chan bool
	DoNotWriteUndoFiles bool
	CB                  CallbackFunctions
}

type NewUnspentOpts struct {
	Dir             string
	Rescan          bool
	VolatimeMode    bool
	UnwindBufferLen uint32
	CB              CallbackFunctions
	AbortNow        *bool
}

func NewUnspentDb(opts *NewUnspentOpts) (db *UnspentDB) {
	//var maxbl_fn string
	db = new(UnspentDB)
	db.dir_utxo = opts.Dir
	db.dir_undo = db.dir_utxo + "undo" + string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 256
	db.CB = opts.CB
	db.abortwritingnow = make(chan bool, 1)
	db.hurryup = make(chan bool, 1)

	os.MkdirAll(db.dir_undo, 0770)

	os.Remove(db.dir_undo + "tmp")
	os.Remove(db.dir_utxo + "UTXO.db.tmp")

	if opts.Rescan {
		db.HashMap = make(map[UtxoKeyType][]byte, UTXO_RECORDS_PREALLOC)
		return
	}

	// Load data form disk
	var k UtxoKeyType
	var cnt_dwn, cnt_dwn_from, perc int
	var le uint64
	var u64, tot_recs uint64
	var info string
	var rd *bufio.Reader
	var of *os.File

	fname := "UTXO.db"

redo:
	of, er := os.Open(db.dir_utxo + fname)
	if er != nil {
		goto fatal_error
	}

	rd = bufio.NewReaderSize(of, 0x100000)

	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}
	db.LastBlockHeight = uint32(u64)

	db.LastBlockHash = make([]byte, 32)
	_, er = rd.Read(db.LastBlockHash)
	if er != nil {
		goto fatal_error
	}
	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}

	//fmt.Println("Last block height", db.LastBlockHeight, "   Number of records", u64)
	cnt_dwn_from = int(u64 / 100)
	perc = 0

	db.HashMap = make(map[UtxoKeyType][]byte, int(u64))
	info = fmt.Sprint("\rLoading ", u64, " transactions from ", fname, " - ")

	for tot_recs = 0; tot_recs < u64; tot_recs++ {
		if opts.AbortNow != nil && *opts.AbortNow {
			break
		}
		le, er = bch.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		er = bch.ReadAll(rd, k[:])
		if er != nil {
			goto fatal_error
		}

		b := malloc(uint32(int(le) - UtxoIdxLen))
		er = bch.ReadAll(rd, b)
		if er != nil {
			goto fatal_error
		}

		// we don't lock RWMutex here as this code is only used during init phase, when no other routines are running
		db.HashMap[k] = b

		if cnt_dwn == 0 {
			fmt.Print(info, perc, "% complete ... ")
			perc++
			cnt_dwn = cnt_dwn_from
		} else {
			cnt_dwn--
		}
	}
	of.Close()

	fmt.Print("\r                                                              \r")

	atomic.StoreUint32(&db.CurrentHeightOnDisk, db.LastBlockHeight)

	return

fatal_error:
	if of != nil {
		of.Close()
	}

	println(er.Error())
	if fname != "UTXO.old" {
		fname = "UTXO.old"
		goto redo
	}
	db.LastBlockHeight = 0
	db.LastBlockHash = nil
	db.HashMap = make(map[UtxoKeyType][]byte, UTXO_RECORDS_PREALLOC)

	return
}

func (db *UnspentDB) save() {
	//var cnt_dwn, cnt_dwn_from, perc int
	var abort, hurryup, check_time bool
	var total_records, current_record, data_progress, time_progress int64

	const save_buffer_min = 0x10000
	const save_buffer_cnt = 100

	os.Rename(db.dir_utxo+"UTXO.db", db.dir_utxo+"UTXO.old")
	data_channel := make(chan []byte, save_buffer_cnt)
	exit_channel := make(chan bool, 1)

	start_time := time.Now()

	db.RWMutex.RLock()

	total_records = int64(len(db.HashMap))

	buf := bytes.NewBuffer(make([]byte, 0, save_buffer_min+0x1000)) // add 4K extra for the last record (it will still be able to grow over it)
	binary.Write(buf, binary.LittleEndian, uint64(db.LastBlockHeight))
	buf.Write(db.LastBlockHash)
	binary.Write(buf, binary.LittleEndian, uint64(total_records))

	// The data is written in a separate process
	// so we can abort without waiting for disk.
	db.lastFileClosed.Add(1)
	go func(fname string) {
		of, er := os.Create(fname)
		if er != nil {
			println("Create file:", er.Error())
			return
		}

		var dat []byte
		var abort, exit bool

		for !exit || len(data_channel) > 0 {
			select {

			case dat = <-data_channel:
				if len(exit_channel) > 0 {
					if abort = <-exit_channel; abort {
						goto exit
					} else {
						exit = true
					}
				}
				of.Write(dat)

			case abort = <-exit_channel:
				if abort {
					goto exit
				} else {
					exit = true
				}
			}
		}
	exit:
		if abort {
			of.Close() // abort
			os.Remove(fname)
		} else {
			of.Close()
			os.Rename(fname, db.dir_utxo+"UTXO.db")
		}
		db.lastFileClosed.Done()
	}(db.dir_utxo + bch.NewUint256(db.LastBlockHash).String() + ".db.tmp")

	for k, v := range db.HashMap {
		if check_time {
			check_time = false
			data_progress = int64((current_record << 20) / total_records)
			time_progress = int64((time.Now().Sub(start_time) << 20) / UTXO_WRITING_TIME_TARGET)
			if data_progress > time_progress {
				select {
				case <-db.abortwritingnow:
					abort = true
					goto finito
				case <-db.hurryup:
					hurryup = true
				case <-time.After(time.Millisecond):
				}
			}
		}

		for len(data_channel) >= cap(data_channel) {
			select {
			case <-db.abortwritingnow:
				abort = true
				goto finito
			case <-db.hurryup:
				hurryup = true
			case <-time.After(time.Millisecond):
			}
		}

		bch.WriteVlen(buf, uint64(UtxoIdxLen+len(v)))
		buf.Write(k[:])
		buf.Write(v)
		if buf.Len() >= save_buffer_min {
			data_channel <- buf.Bytes()
			buf = bytes.NewBuffer(make([]byte, 0, save_buffer_min+0x1000)) // add 4K extra for the last record
		}

		if !hurryup {
			current_record++
			if (current_record & 0x3f) == 0 {
				check_time = true
			}
		}
	}
finito:
	db.RWMutex.RUnlock()

	if !abort && buf.Len() > 0 {
		data_channel <- buf.Bytes()
	}
	exit_channel <- abort

	if !abort {
		db.DirtyDB.Clr()
		//println("utxo written OK in", time.Now().Sub(start_time).String(), timewaits)
		atomic.StoreUint32(&db.CurrentHeightOnDisk, db.LastBlockHeight)
	}
	db.WritingInProgress.Clr()
	db.writingDone.Done()
}

// Commit the given add/del transactions to UTXO and Unwind DBs
func (db *UnspentDB) CommitBlockTxs(changes *BchBlockChanges, blhash []byte) (e error) {
	undo_fn := fmt.Sprint(db.dir_undo, changes.Height)

	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortWriting()

	if changes.UndoData != nil {
		bu := new(bytes.Buffer)
		bu.Write(blhash)
		if changes.UndoData != nil {
			for _, xx := range changes.UndoData {
				bin := xx.Serialize(true)
				bch.WriteVlen(bu, uint64(len(bin)))
				bu.Write(bin)
			}
		}
		ioutil.WriteFile(db.dir_undo+"tmp", bu.Bytes(), 0666)
		os.Rename(db.dir_undo+"tmp", undo_fn)
	}

	db.commit(changes)

	if db.LastBlockHash == nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.Height > db.UnwindBufLen {
		os.Remove(fmt.Sprint(db.dir_undo, changes.Height-db.UnwindBufLen))
	}

	db.DirtyDB.Set()
	return
}

func (db *UnspentDB) UndoBlockTxs(bl *bch.BchBlock, newhash []byte) {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortWriting()

	for _, tx := range bl.Txs {
		lst := make([]bool, len(tx.TxOut))
		for i := range lst {
			lst[i] = true
		}
		db.del(tx.Hash.Hash[:], lst)
	}

	fn := fmt.Sprint(db.dir_undo, db.LastBlockHeight)
	var addback []*UtxoRec

	if _, er := os.Stat(fn); er != nil {
		fn += ".tmp"
	}

	dat, er := ioutil.ReadFile(fn)
	if er != nil {
		panic(er.Error())
	}

	off := 32 // ship the block hash
	for off < len(dat) {
		le, n := bch.VLen(dat[off:])
		off += n
		qr := FullUtxoRec(dat[off : off+le])
		off += le
		addback = append(addback, qr)
	}

	for _, tx := range addback {
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(tx)
		}

		var ind UtxoKeyType
		copy(ind[:], tx.TxID[:])
		db.RWMutex.RLock()
		v := db.HashMap[ind]
		db.RWMutex.RUnlock()
		if v != nil {
			oldrec := NewUtxoRec(ind, v)
			for a := range tx.Outs {
				if tx.Outs[a] == nil {
					tx.Outs[a] = oldrec.Outs[a]
				}
			}
		}
		db.RWMutex.Lock()
		db.HashMap[ind] = malloc_and_copy(tx.Bytes())
		db.RWMutex.Unlock()
	}

	os.Remove(fn)
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)
	db.DirtyDB.Set()
}

// Call it when the main thread is idle
func (db *UnspentDB) Idle() bool {
	if db.volatimemode {
		return false
	}

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	if db.DirtyDB.Get() && db.LastBlockHeight-atomic.LoadUint32(&db.CurrentHeightOnDisk) > UTXO_SKIP_SAVE_BLOCKS {
		return db.Save()
	}

	return false
}

func (db *UnspentDB) Save() bool {
	if db.WritingInProgress.Get() {
		return false
	}
	db.WritingInProgress.Set()
	db.writingDone.Add(1)
	go db.save() // this one will call db.writingDone.Done()
	return true
}

func (db *UnspentDB) HurryUp() {
	select {
	case db.hurryup <- true:
	default:
	}
}

// Flush the data and close all the files
func (db *UnspentDB) Close() {
	db.volatimemode = false
	if db.DirtyDB.Get() {
		db.HurryUp()
		db.Save()
	}
	db.writingDone.Wait()
	db.lastFileClosed.Wait()
}

// Get given unspent output
func (db *UnspentDB) UnspentGet(po *bch.TxPrevOut) (res *bch.TxOut) {
	var ind UtxoKeyType
	var v []byte
	copy(ind[:], po.Hash[:])

	db.RWMutex.RLock()
	v = db.HashMap[ind]
	db.RWMutex.RUnlock()
	if v != nil {
		res = OneUtxoRec(ind, v, po.Vout)
	}

	return
}

// Returns true if gived TXID is in UTXO
func (db *UnspentDB) TxPresent(id *bch.Uint256) (res bool) {
	var ind UtxoKeyType
	copy(ind[:], id.Hash[:])
	db.RWMutex.RLock()
	_, res = db.HashMap[ind]
	db.RWMutex.RUnlock()
	return
}

func (db *UnspentDB) del(hash []byte, outs []bool) {
	var ind UtxoKeyType
	copy(ind[:], hash)
	db.RWMutex.RLock()
	v := db.HashMap[ind]
	db.RWMutex.RUnlock()
	if v == nil {
		return // no such txid in UTXO (just ignorde delete request)
	}
	rec := NewUtxoRec(ind, v)
	if db.CB.NotifyTxDel != nil {
		db.CB.NotifyTxDel(rec, outs)
	}
	var anyout bool
	for i, rm := range outs {
		if rm {
			rec.Outs[i] = nil
		} else if rec.Outs[i] != nil {
			anyout = true
		}
	}
	db.RWMutex.Lock()
	if anyout {
		db.HashMap[ind] = malloc_and_copy(rec.Bytes())
	} else {
		delete(db.HashMap, ind)
	}
	db.RWMutex.Unlock()
	free(v)
}

func (db *UnspentDB) commit(changes *BchBlockChanges) {
	// Now aplly the unspent changes
	for _, rec := range changes.AddList {
		var ind UtxoKeyType
		copy(ind[:], rec.TxID[:])
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(rec)
		}
		db.RWMutex.Lock()
		db.HashMap[ind] = malloc_and_copy(rec.Bytes())
		db.RWMutex.Unlock()
	}
	for k, v := range changes.DeledTxs {
		db.del(k[:], v)
	}
}

func (db *UnspentDB) AbortWriting() {
	db.Mutex.Lock()
	db.abortWriting()
	db.Mutex.Unlock()
}

func (db *UnspentDB) abortWriting() {
	if db.WritingInProgress.Get() {
		db.abortwritingnow <- true
		db.writingDone.Wait()
		select {
		case <-db.abortwritingnow:
		default:
		}
	}
}

func (db *UnspentDB) UTXOStats() (s string) {
	var outcnt, sum, sumcb uint64
	var totdatasize, unspendable, unspendable_recs, unspendable_bytes uint64

	db.RWMutex.RLock()

	lele := len(db.HashMap)

	for k, v := range db.HashMap {
		totdatasize += uint64(len(v) + 8)
		rec := NewUtxoRecStatic(k, v)
		var spendable_found bool
		for _, r := range rec.Outs {
			if r != nil {
				outcnt++
				sum += r.Value
				if rec.Coinbase {
					sumcb += r.Value
				}
				if len(r.PKScr) > 0 && r.PKScr[0] == 0x6a {
					unspendable++
					unspendable_bytes += uint64(8 + len(r.PKScr))
				} else {
					spendable_found = true
				}
			}
		}
		if !spendable_found {
			unspendable_recs++
		}
	}

	db.RWMutex.RUnlock()

	s = fmt.Sprintf("UNSPENT: %.8f BCH in %d outs from %d txs. %.8f BCH in coinbase.\n",
		float64(sum)/1e8, outcnt, lele, float64(sumcb)/1e8)
	s += fmt.Sprintf(" TotalData:%.1fMB  MaxTxOutCnt:%d  DirtyDB:%t  Writing:%t  Abort:%t\n",
		float64(totdatasize)/1e6, len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", bch.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Unspendable outputs: %d (%dKB)  txs:%d\n",
		unspendable, unspendable_bytes>>10, unspendable_recs)

	return
}

// Return DB statistics
func (db *UnspentDB) GetStats() (s string) {
	db.RWMutex.RLock()
	hml := len(db.HashMap)
	db.RWMutex.RUnlock()

	s = fmt.Sprintf("UNSPENT: %d records. MaxTxOutCnt:%d  DirtyDB:%t  Writing:%t  Abort:%t\n",
		hml, len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", bch.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	return
}

func (db *UnspentDB) PurgeUnspendable(all bool) {
	var unspendable_txs, unspendable_recs uint64
	db.Mutex.Lock()
	db.abortWriting()

	db.RWMutex.Lock()

	for k, v := range db.HashMap {
		rec := NewUtxoRecStatic(k, v)
		var spendable_found bool
		var record_removed uint64
		for idx, r := range rec.Outs {
			if r != nil {
				if len(r.PKScr) > 0 && r.PKScr[0] == 0x6a {
					unspendable_recs++
					if all {
						rec.Outs[idx] = nil
						record_removed++
					}
				} else {
					spendable_found = true
				}
			}
		}
		if !spendable_found {
			free(v)
			delete(db.HashMap, k)
			unspendable_txs++
		} else if record_removed > 0 {
			free(v)
			db.HashMap[k] = malloc_and_copy(rec.Serialize(false))
			unspendable_recs += record_removed
		}
	}
	db.RWMutex.Unlock()

	db.Mutex.Unlock()

	fmt.Println("Purged", unspendable_txs, "transactions and", unspendable_recs, "extra records")
}
