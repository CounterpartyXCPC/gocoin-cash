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

// File:		target.go
// Description:	Bictoin Cash Target Package

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

package bch

import (
	"math/big"
)

func SetCompact(nCompact uint32) (res *big.Int) {
	size := nCompact >> 24
	neg := (nCompact & 0x00800000) != 0
	word := nCompact & 0x007fffff
	if size <= 3 {
		word >>= 8 * (3 - size)
		res = big.NewInt(int64(word))
	} else {
		res = big.NewInt(int64(word))
		res.Lsh(res, uint(8*(size-3)))
	}
	if neg {
		res.Neg(res)
	}
	return res
}

func GetDifficulty(bits uint32) (diff float64) {
	shift := int(bits>>24) & 0xff
	diff = float64(0x0000ffff) / float64(bits&0x00ffffff)
	for shift < 29 {
		diff *= 256.0
		shift++
	}
	for shift > 29 {
		diff /= 256.0
		shift--
	}
	return
}

func GetBchDifficulty(bits uint32) (diff float64) {

	shift := int(bits>>24) & 0xff
	diff = float64(0x0000ffff) / float64(bits&0x00ffffff)
	for shift < 29 {
		diff *= 256.0
		shift++
	}
	for shift > 29 {
		diff /= 256.0
		shift--
	}
	return

}

func GetCompact(b *big.Int) uint32 {

	size := uint32(len(b.Bytes()))
	var compact uint32

	if size <= 3 {
		compact = uint32(b.Int64() << uint(8*(3-size)))
	} else {
		b = new(big.Int).Rsh(b, uint(8*(size-3)))
		compact = uint32(b.Int64())
	}

	// The 0x00800000 bit denotes the sign.
	// Thus, if it is already set, divide the mantissa by 256 and increase the exponent.
	if (compact & 0x00800000) != 0 {
		compact >>= 8
		size++
	}
	compact |= size << 24
	if b.Cmp(big.NewInt(0)) < 0 {
		compact |= 0x00800000
	}
	return compact
}

func CheckProofOfWork(hash *Uint256, bits uint32) bool {
	return hash.BigInt().Cmp(SetCompact(bits)) <= 0
}
