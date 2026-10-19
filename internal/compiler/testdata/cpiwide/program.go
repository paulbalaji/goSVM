// Package cpiwide issues one CPI from a program built with -max-accounts 64,
// naming accounts by explicit incoming indices (any of 0..63). Accounts: 0 the
// cpi-limits callee, 1..4 the PDAs of seeds ("cpi-limits", group) under this
// program. Instruction: groups, plain, metas, one bump per group, then one
// incoming index per meta; the first groups metas are the PDA signers. The
// callee receives [metas, groups, plain] (examples/cpi-limits/callee.c.txt).
package cpiwide

import (
	"gosvm/sdk/cpi"
	"gosvm/sdk/pda"
	"gosvm/solana"
)

const ErrInput uint64 = 4000

var label = [10]byte{'c', 'p', 'i', '-', 'l', 'i', 'm', 'i', 't', 's'}

func Process(c solana.Context) uint64 {
	ix := solana.Instruction(c)
	if len(ix) < 3 {
		return ErrInput
	}
	groups, plain, n := uint64(ix[0]), ix[1], uint64(ix[2])
	if groups > 4 || n > 64 || groups > n || uint64(len(ix)) != 3+groups+n {
		return ErrInput
	}
	metas := new(cpi.Metas)
	for i := uint64(0); i < n; i++ {
		if code := metas.Add(uint64(ix[3+groups+i]), false, i < groups); code != 0 {
			return code
		}
	}
	signers := new(cpi.Signers)
	for g := uint64(0); g < groups; g++ {
		seeds := new(pda.Seeds)
		if seeds.AddBytes(label[:]) != 0 || seeds.AddByte(byte(g)) != 0 || seeds.AddByte(ix[3+g]) != 0 {
			return pda.ErrSeeds
		}
		if code := signers.Add(seeds); code != 0 {
			return code
		}
	}
	var data [3]byte
	data[0], data[1], data[2] = byte(n), byte(groups), plain
	return cpi.Invoke(c, 0, metas, data[:], signers)
}
