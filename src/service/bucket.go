package service

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/service/bucket"
)

// bucket for storing the balance of deposit and ico coin
type coinValueBucket struct {
	bkt *bucket.Bucket
}

func newCoinValueBucket(name []byte, db *bolt.DB) *coinValueBucket {
	bkt, err := bucket.New(name, db)
	if err != nil {
		panic(err)
	}
	return &coinValueBucket{
		bkt: bkt,
	}
}

func (cvb *coinValueBucket) get(address string) (coinValue, bool) {
	v := cvb.bkt.Get([]byte(address))
	var cv coinValue
	if v == nil {
		return cv, false
	}
	if err := json.NewDecoder(bytes.NewReader(v)).Decode(&cv); err != nil {
		return cv, false
	}
	return cv, true
}

func (cvb *coinValueBucket) put(cv coinValue) error {
	v, err := json.Marshal(cv)
	if err != nil {
		return err
	}
	return cvb.bkt.Put([]byte(cv.Address), v)
}

func (cvb *coinValueBucket) getAllBalances() (map[string]int64, error) {
	vm := make(map[string]int64)
	rawMap := cvb.bkt.GetAll()

	for k, v := range rawMap {
		// decode v into  coinValue struct
		cv := coinValue{}
		if err := json.NewDecoder(bytes.NewReader(v)).Decode(&cv); err != nil {
			return nil, err
		}

		vm[k.(string)] = cv.Balance
	}
	return vm, nil
}

func (cvb *coinValueBucket) changeBalance(addr string, value int64) error {
	return cvb.bkt.DB().Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(cvb.bkt.Name)
		if bkt == nil {
			return fmt.Errorf("%s bucket does not exist", cvb.bkt.Name)
		}

		v := bkt.Get([]byte(addr))
		var cv coinValue
		if err := json.NewDecoder(bytes.NewReader(v)).Decode(&cv); err != nil {
			return err
		}

		cv.Balance += value
		nv, err := json.Marshal(cv)
		if err != nil {
			return err
		}
		return bkt.Put([]byte(addr), nv)
	})
}

type exchangeLogBucket struct {
	bkt *bucket.Bucket
}

func newExchangeLogBucket(name []byte, db *bolt.DB) *exchangeLogBucket {
	bkt, err := bucket.New(name, db)
	if err != nil {
		panic(err)
	}
	return &exchangeLogBucket{bkt: bkt}
}

func (elb *exchangeLogBucket) put(log *daemon.ExchangeLog) error {
	return elb.bkt.DB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(elb.bkt.Name)
		id, err := b.NextSequence()
		if err != nil {
			return err
		}

		log.ID = int(id)
		d, err := json.Marshal(log)
		if err != nil {
			return err
		}

		return b.Put(itob(log.ID), d)
	})
}

func (elb *exchangeLogBucket) get(start, end int) (logs []daemon.ExchangeLog, err error) {
	if start > end {
		return []daemon.ExchangeLog{}, errors.New("start must <= end")
	}

	err = elb.bkt.DB().View(func(tx *bolt.Tx) error {
		c := tx.Bucket(elb.bkt.Name).Cursor()
		for k, v := c.Seek(itob(start)); k != nil && btoi(k) <= end; k, v = c.Next() {
			var log daemon.ExchangeLog
			if err := json.Unmarshal(v, &log); err != nil {
				return err
			}
			logs = append(logs, log)
		}
		return nil
	})
	return
}

func (elb *exchangeLogBucket) update(id int, f func(*daemon.ExchangeLog)) error {
	return elb.bkt.DB().Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(elb.bkt.Name)
		v := bkt.Get(itob(id))
		var log daemon.ExchangeLog
		if err := json.Unmarshal(v, &log); err != nil {
			return err
		}
		f(&log)
		d, err := json.Marshal(log)
		if err != nil {
			return err
		}
		return bkt.Put(itob(id), d)
	})
}

func (elb *exchangeLogBucket) len() int {
	return elb.bkt.Len()
}

// itob returns an 8-byte big endia representation of v.
func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func btoi(b []byte) int {
	return int(binary.BigEndian.Uint64(b))
}

// unconfirmedTxids records unconfirmed txid and corresonding exchange log id
type unconfirmedTxids struct {
	value *bucket.Bucket
}

func newUnconfirmedTxids(name []byte, db *bolt.DB) *unconfirmedTxids {
	bkt, err := bucket.New(name, db)
	if err != nil {
		panic(err)
	}

	return &unconfirmedTxids{
		value: bkt,
	}
}

func (utx *unconfirmedTxids) put(txid string, logid int) error {
	return utx.value.Put([]byte(txid), itob(logid))
}

func (utx *unconfirmedTxids) get(txid string) (int, bool) {
	v := utx.value.Get([]byte(txid))
	if v == nil {
		return 0, false
	}

	return btoi(v), true
}

func (utx *unconfirmedTxids) forEach(f func(txid string, logid int)) error {
	return utx.value.ForEach(func(k, v []byte) error {
		f(string(k), btoi(v))
		return nil
	})
}

func (utx *unconfirmedTxids) delete(txid string) error {
	return utx.value.Delete([]byte(txid))
}
