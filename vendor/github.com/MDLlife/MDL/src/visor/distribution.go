package visor

import "github.com/MDLlife/MDL/src/coin"

const (
	// MaxCoinSupply is the maximum supply of skycoins
	MaxCoinSupply uint64 = 1e9 // 1,000,000,000

	// DistributionAddressesTotal is the number of distribution addresses
	DistributionAddressesTotal uint64 = 100

	// DistributionAddressInitialBalance is the initial balance of each distribution address
	DistributionAddressInitialBalance uint64 = MaxCoinSupply / DistributionAddressesTotal

	// InitialUnlockedCount is the initial number of unlocked addresses
	InitialUnlockedCount uint64 = 56

	// UnlockAddressRate is the number of addresses to unlock per unlock time interval
	UnlockAddressRate uint64 = 24

	// UnlockTimeInterval is the distribution address unlock time interval, measured in seconds
	// Once the InitialUnlockedCount is exhausted,
	// UnlockAddressRate addresses will be unlocked per UnlockTimeInterval
	UnlockTimeInterval uint64 = 60 * 60 * 24 * 365 // 1 year
)

func init() {
	if MaxCoinSupply%DistributionAddressesTotal != 0 {
		panic("MaxCoinSupply should be perfectly divisible by DistributionAddressesTotal")
	}
}

// GetDistributionAddresses returns a copy of the hardcoded distribution addresses array.
// Each address has 10,000,000 coins. There are 100 addresses.
func GetDistributionAddresses() []string {
	addrs := make([]string, len(distributionAddresses))
	for i := range distributionAddresses {
		addrs[i] = distributionAddresses[i]
	}
	return addrs
}

// GetUnlockedDistributionAddresses returns distribution addresses that are unlocked, i.e. they have spendable outputs
func GetUnlockedDistributionAddresses() []string {
	// The first InitialUnlockedCount (100) addresses are unlocked by default.
	// Subsequent addresses will be unlocked at a rate of UnlockAddressRate (5) per year,
	// after the InitialUnlockedCount (25) addresses have no remaining balance.
	// The unlock timer will be enabled manually once the
	// InitialUnlockedCount (25) addresses are distributed.

	// NOTE: To have automatic unlocking, transaction verification would have
	// to be handled in visor rather than in coin.Transactions.Visor(), because
	// the coin package is agnostic to the state of the blockchain and cannot reference it.
	// Instead of automatic unlocking, we can hardcode the timestamp at which the first 30%
	// is distributed, then compute the unlocked addresses easily here.

	addrs := make([]string, InitialUnlockedCount)
	for i := range distributionAddresses[:InitialUnlockedCount] {
		addrs[i] = distributionAddresses[i]
	}
	return addrs
}

// GetLockedDistributionAddresses returns distribution addresses that are locked, i.e. they have unspendable outputs
func GetLockedDistributionAddresses() []string {
	// TODO -- once we reach 30% distribution, we can hardcode the
	// initial timestamp for releasing more coins
	addrs := make([]string, DistributionAddressesTotal-InitialUnlockedCount)
	for i := range distributionAddresses[InitialUnlockedCount:] {
		addrs[i] = distributionAddresses[InitialUnlockedCount+uint64(i)]
	}
	return addrs
}

// TransactionIsLocked returns true if the transaction spends locked outputs
func TransactionIsLocked(inUxs coin.UxArray) bool {
	lockedAddrs := GetLockedDistributionAddresses()
	lockedAddrsMap := make(map[string]struct{})
	for _, a := range lockedAddrs {
		lockedAddrsMap[a] = struct{}{}
	}

	for _, o := range inUxs {
		uxAddr := o.Body.Address.String()
		if _, ok := lockedAddrsMap[uxAddr]; ok {
			return true
		}
	}

	return false
}

var distributionAddresses = [DistributionAddressesTotal]string{
	"czQ5CFysUTYDNqB2usjjdvEKy8Y5Wpewyh",
	"2dAGNzdgCPv4Shbe4A7hkNQEJL4jUF3ehcF",
	"mRyYjpnfFUeao5absoCwCe9z4PAJfnEW7r",
	"gWw3pL7okUQe1m1jBnpeiFjbBN9YA1mtbZ",
	"xJeu6Auwb54xeQfMCVJgXht3N44khD5eQW",
	"2g8a5ZGoqbHk6Hr42GBaXcqFK3gvy2GZvhf",
	"CdQ2SXuuZ6oNvivX1GpWoZcjne61wGFUaV",
	"2K6VWQGvkTHAgmh6yWeu8K4tXqahMrjxa4i",
	"2LUgXJK78VB7tcARHQAMeXsZTba9mzant2k",
	"298PpFFEeYoZAtJexfHmywGYtPNUg1UKK2A",
	"24C5KaENz4VvmfG1xv4HMPQU69pcJMwXAKZ",
	"2Rjr6wVyncNdsiVk4ZULcx8AiM7rbuiY7yt",
	"ASpdMF7B4jwtqYpmz54bBQybXFHTW9oMLS",
	"M7iPe2rwvZWhsuAUyXjG9tdze1cpyL7v7m",
	"zws2msPdHGp4VAxMLGHbUJwgbZdCQ4xGoT",
	"aXvixs8fdgSnQDSdabce3eYaXQNEr1HZV1",
	"2BTpkFTEJ2Txv8i99f9WNZNcwECZmQVCwbk",
	"XMVKNxauGpDfUWTN45LKuqWV1A6yPKdyhw",
	"Frzn1E2HGV7WkcTWSGtH8tht1QSwncAFjc",
	"W4q8U5CXozkTfFQNP9nwCrrrcDbiyGEXYP",
	"2KC3m6YbX4jf8n42nFY5D7k1ws4fWzAPhrn",
	"2N3d2VZCp6EpH1H8AZ5kmNQq4dS4zo1u7Dw",
	"hGeqAhSSGGm4VoH8prefVwHE5t55ePncrX",
	"2cvs1A5vwMnfFSbMotNS4Rqrp5ZZJYP7rDM",
	"2mW5PmgXp1RNBK3A7JZgz7AGcPr4crn9rto",
	"7Sb5PMQYchRofXjPgjrotMqk1YcFuizdr8",
	"bdjVMJ5i2kAGAh33ZrsKV2yjZxV6NsVGb1",
	"2NrzzrW9MgXPyVrHYqHD2HYLEmUQJJ2kNHf",
	"oYosFCapLA61fczjLbgU9wN2F69BzwwTR8",
	"2fk5zwsKz7tqKSJcT3zbzSyPfEr7fL3aiWo",
	"2Ze8QePcHxX8ehYfBKp3oWJGXdoScTyzxkT",
	"JksveJVfR5ronVt64ifeip2Gmc6mTk4qEq",
	"2HYZmUUaCB1roZt5ZWbUdSX5mYhSdeWotwu",
	"2DhBEAarac7m3ESMoyYVt6iVPJ8VNkk5WAr",
	"cX9PVpSN6mB3uVNJa6nk8qZnaj2mvrk9Ke",
	"26V697Dm8wCPcvn8z6Lx1RMRP1gMKSMwZhz",
	"BWJ4hvVrBuPuBU7YX9ufXyRFZvAURCkNhF",
	"2eDpyQSS5pimLRNbfKn31F34S9QHDX93ojA",
	"sDNVkAntVKY32vJ99rhgzUnjPz9UBGvdjC",
	"R1FgfgENJNX9XTvVC3wYvwNuWDmgaZbbSW",
	"mPr3XdR1rCF4Rwco1YTE3fnJkbHrmi8L5Q",
	"z9gC7LG5HFgdiCyj5s7WiSvq4M9s4Ar2H4",
	"WLnPhwRrott78ur57E24Ns9pTubaJqBrW1",
	"qdq48BWXxboGjrV9PJj5njK5WaAwBMeUUN",
	"2dqLJoAUQiPx9njaFpE4ano9nvMGYSTpJN4",
	"2HfVWrjubFRJy1MVShgsv7MckPbGfL17Y2T",
	"BV9YUZg88oeyHSNoJNDxAUAQqg3A73esHo",
	"Uv9VzjT4tkA2CU15bsFiPiJtjyQhUrkABG",
	"wpmKzZTJKbWBR4ZcA1judzjDQrqur6PCEm",
	"p8aE795QKLsLHFfZba1YWpekZxDco99cTu",
	"PQdzt6fvhK2dsqowNMFQPz9aJ1pK8iSqzW",
	"2LsZFGXTxK69wht3pQtV72VBfvmAFrCYq9C",
	"18AgYwA3FBRgjLjsQ552KXvxYnWSM1ek3j",
	"BC9dZmuudgfr1ZRqVKMYyCz9tuDATbkNju",
	"5sxt6PDEjM8DkMkZ8z16eSmspbAzBs8kDn",
	"KTpqcH9iW1xvZNHV26jmUJQeinnZQXfjoi",
	"2XNPgq7jRSPSvVDxzAWULpekTqYdWQHT7Eg",
	"Tfxef8dF7hKipzopgyJ8w31S8zzKGq5Mgj",
	"22mbJCQboNHYXe7uV7RYSFejR4dSzTJa5sX",
	"gy4huwqBjtRmkFVPAjVt8ajvPPsAogUSnF",
	"2MfM8Z2cadrqbKzjgWs1NdU3WzSidTcbgqh",
	"pgysZ9UQXyahRNAgiK2N3vWhj8tXz1bmEc",
	"KE1En9rWfDLsFJ6gHtgq5zM7c65ybqTFSX",
	"2Fyh2BkTqtPCEkBiCD3RG2Jqtr7rJUEYbcq",
	"umofLhswKZ2KiWTPcGEbcsw5qyhTZydZWh",
	"FRBbWo7EYLqVciVedwu2d7GVEbP4GnTVww",
	"236j2jsVNHmtHKeGxqbUzVkSP3GWXs2JCQx",
	"2jC2H9qm82LkKVfKQqLK7aDSGm9UzTJq6Jy",
	"232rP89hcUcmN6PxiJoqEHvApM93qpGkLG4",
	"HNEjd5PkgSTRT8J1wkoJogqZx3CvWuuFsF",
	"2GeELJpt8zTQYpX7mdjUqXY6jL15ECkULct",
	"SbEtQEp8f6mt4AAnGRvhouyUaPbJWCj2dj",
	"uKJiwSEVJ6ygLKC2NusrSbAkR7qroqDXyY",
	"2RZq4AdSfGNPc4gvV9kmg7BeWzdWZY7MHSX",
	"28MdXYcghLqh77v1bn143uerJhQVfYijejU",
	"2AU6dqKPkqE1rkiZFNYwm4kg2sKWi5Vw8xC",
	"2Grpx1D2SwHw8oZL5c9MgdkppzBq4kSJS8E",
	"Zk2VYvBUJeoJWL5fK8NerUeWW3we8ucKXs",
	"23EKv4CUZBG7S7t7wbex6yTCvqfmvrgvEPm",
	"2PJxrtQKN8h4MwoePHpypntfDgu93Rpnd39",
	"pG9Hi9RyXU5CSNdYHmsYefeU6cVTiWVzPm",
	"fJdyQV58wCoE6boekQEvK2kpoXL47YfMKZ",
	"vnfsK3R7fY1ReQy2Fyp7NdacdQPubfpVvh",
	"2VDtZJ96PpLzFKrBEChW7R7i7UfZQzRxPbw",
	"FzyQg4YwsCAwByBwWMmr8H4yJ6JTuZXS4z",
	"WPTST2QRGAxCkEBmaLYbBba9Jj4NnzkAVE",
	"bNJtnBtzPh7SWQPDHt9x9fnWV69kHoh7mi",
	"2giFWTbYMUcFfYjBCK4ntdh3C4974Fi4rXs",
	"2kZy49WY6pqaBzDfM9i2Mucx3KGNiY48459",
	"oPRqcrZXK6fcDxE87DQLRqa7CpzwgcDi57",
	"Dp4hj2ueG8d5D2TQhoJ6R8HR7BdsCqoK7y",
	"2ksmbJ7g94vrPuj1epcoktgS3azQRstx5Ly",
	"NAipXTCA7s7TsuLF5vYfo1soM4qsDdVqWe",
	"nv1t1pj7vikFAJ89hKPGZwGArMR3tRAk6o",
	"2PxQpePbDqC9wWVDH17RUypiJiS56sSZT4S",
	"C5DxWe6onmCPY7LuxXgGjxqRny5kPmPzAV",
	"Q7Bpt1xNYki4GNs5wPNJfHoacb5nmqgGip",
	"2e1XQxpAnySE4qvuPfiUmchxynkSfFVUdtJ",
	"TpDAYoV9qNhC4Q3hwuk1mU2rRhFkZV1UFj",
	"78Av9fW9wHqA2z95dG2NxehoeqQb2vWJ3G",
}
