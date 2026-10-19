package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gosvm/internal/sbftest"
	target "gosvm/internal/targets"
	"gosvm/internal/testvm"
	"gosvm/sdk/fixture"
)

// cpiWidePlain is the number of distinct plain accounts. With the callee, the
// four PDAs, the fee payer and the program, 29 keys and 64 descriptors fit one
// legacy packet (checked below), so 64 metas name at most 26 distinct accounts.
const cpiWidePlain = 22

// cpiWideFixtures builds the 64-descriptor instruction (callee, PDAs, plain
// accounts, then aliases of the plain accounts up to index 63) and cases whose
// metas prefer the highest incoming index for each key, so most CPI metas name
// descriptors at 32 and above.
func cpiWideFixtures(t *testing.T, calleeSHA string) (wide, narrow []byte) {
	t.Helper()
	build := func() (*fixture.Fixture, fixture.Account, []fixture.AccountMeta, []byte) {
		f := fixture.New("cpi wide")
		callee := f.Program("callee", f.KeyFor("callee"), "callee.so", calleeSHA)
		accounts := []fixture.AccountMeta{{Key: callee.Key}}
		var bumps []byte
		for g := 0; g < 4; g++ {
			p := f.PDA(fmt.Sprintf("signer-%d", g), []byte("cpi-limits"), []byte{byte(g)})
			accounts = append(accounts, fixture.AccountMeta{Key: p.Key})
			bumps = append(bumps, p.Bump)
		}
		var plain []fixture.Account
		for j := 0; j < cpiWidePlain; j++ {
			plain = append(plain, f.Account(fmt.Sprintf("plain-%02d", j), fixture.Wallet(1_000_000)))
			accounts = append(accounts, fixture.AccountMeta{Key: plain[j].Key})
		}
		for j := 0; len(accounts) < 64; j++ {
			accounts = append(accounts, fixture.AccountMeta{Key: plain[j%cpiWidePlain].Key})
		}
		return f, callee, accounts, bumps
	}
	// candidates[k] lists the incoming indices holding plain key k, highest first.
	candidates := make([][]byte, cpiWidePlain)
	for i := 5; i < 64; i++ {
		k := (i - 5) % cpiWidePlain
		candidates[k] = append(candidates[k], byte(i))
	}
	for k := range candidates {
		sort.Slice(candidates[k], func(a, b int) bool { return candidates[k][a] > candidates[k][b] })
	}
	ixData := func(bumps []byte, groups, n int) []byte {
		data := append([]byte{byte(groups), cpiWidePlain, byte(n)}, bumps[:groups]...)
		for i := 0; i < groups; i++ {
			data = append(data, byte(1+i))
		}
		for i := groups; i < n; i++ {
			k, r := (i-groups)%cpiWidePlain, (i-groups)/cpiWidePlain
			data = append(data, candidates[k][r%len(candidates[k])])
		}
		return data
	}
	packet := func(accounts []fixture.AccountMeta, data []byte) {
		keys := map[[32]byte]bool{}
		for _, a := range accounts {
			keys[a.Key] = true
		}
		// one signature, header, keys plus the fee payer and the program,
		// blockhash, one instruction; compact-u16 lengths are one byte.
		if n := 1 + 64 + 3 + 1 + 32*(len(keys)+2) + 32 + 1 + 1 + 1 + len(accounts) + 1 + len(data); n > 1232 {
			t.Fatalf("instruction needs %d bytes, above the 1,232-byte packet", n)
		}
	}
	cases := []struct {
		name      string
		groups, n int
	}{
		{"wide-26-metas-4-groups", 4, 26},
		{"wide-33-metas", 0, 33},
		{"wide-64-metas-4-groups", 4, 64},
	}
	f, callee, accounts, bumps := build()
	for _, c := range cases {
		data := ixData(bumps, c.groups, c.n)
		packet(accounts, data)
		f.Scenario(c.name).Step(fixture.Instruction{Accounts: accounts, Data: data}).ExpectInvocations(callee.Key, 1)
	}
	var err error
	if wide, err = f.JSON(); err != nil {
		t.Fatal(err)
	}
	// Without -max-accounts 64 the same 64-descriptor instruction is 1001.
	g, gCallee, gAccounts, gBumps := build()
	g.Scenario("narrow-64-descriptors-fails").Step(fixture.Instruction{Accounts: gAccounts, Data: ixData(gBumps, 4, 64)}).
		Fails(1001).ExpectInvocations(gCallee.Key, 0)
	if narrow, err = g.JSON(); err != nil {
		t.Fatal(err)
	}
	return wide, narrow
}

// TestCPIWideAccountsSBF checks that the opt-in 64-account decoder
// (gap2-accounts64) and the raised CPI bounds (cpi-raise-limits) work
// together: a program built with MaxAccounts = WideAccounts receives 64
// descriptors and sends 26, 33 and 64 metas with up to 4 PDA signer groups,
// most naming incoming indices 32..63, to a callee that checks its account
// count, signers and every duplicate position. Distinct accounts stay at 26:
// a legacy packet cannot carry more keys with 64 descriptors.
func TestCPIWideAccountsSBF(t *testing.T) {
	if testing.Short() {
		t.Skip("SBF runs are skipped in -short mode")
	}
	llvm := modernLLVM()
	if llvm == "" {
		t.Skip("pinned platform-tools v1.57 backend required")
	}
	for _, profile := range []string{target.Fogo, target.Solana} {
		t.Run(profile, func(t *testing.T) {
			runner, err := testvm.RunnerFor("", profile)
			if err != nil {
				t.Skipf("runner unavailable: %v", err)
			}
			dir := t.TempDir()
			callee := buildCPICallee(t, llvm, profile, dir)
			wide, narrow := cpiWideFixtures(t, fmt.Sprintf("%x", sha256.Sum256(mustRead(t, callee))))
			p, err := target.Resolve(profile)
			if err != nil {
				t.Fatal(err)
			}
			for _, build := range []struct {
				name     string
				max      int
				fixtures []byte
				cases    int
			}{{"max-accounts-64", WideAccounts, wide, 3}, {"default", 0, narrow, 1}} {
				program, err := readProgramWith("testdata/cpiwide", BuildOptions{MaxAccounts: build.max})
				if err != nil {
					t.Fatal(err)
				}
				program.SDK = 2
				elf := filepath.Join(dir, build.name+".so")
				if err = buildProgram(program, elf, llvm, profile); err != nil {
					t.Fatal(err)
				}
				fixturePath := filepath.Join(dir, build.name+".json")
				if err = os.WriteFile(fixturePath, build.fixtures, 0644); err != nil {
					t.Fatal(err)
				}
				report := filepath.Join(dir, build.name+"-results.json")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				var log strings.Builder
				err = sbftest.RunGeneral(ctx, elf, fixturePath, report, &log, sbftest.FastOptions{Runner: runner, Profile: p.Name, PlatformTools: p.Tools})
				cancel()
				if err != nil {
					t.Fatalf("%s: %v\n%s", build.name, err, log.String())
				}
				var parsed struct {
					Cases []sbftest.FixtureCaseResult `json:"cases"`
				}
				if err = json.Unmarshal(mustRead(t, report), &parsed); err != nil {
					t.Fatal(err)
				}
				if len(parsed.Cases) != build.cases {
					t.Fatalf("%s: ran %d of %d cases", build.name, len(parsed.Cases), build.cases)
				}
				for _, c := range parsed.Cases {
					t.Logf("%s %s: %d CU", build.name, c.Name, c.Steps[0].CU)
				}
			}
		})
	}
}
