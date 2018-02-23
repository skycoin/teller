package main

import (
	"errors"
	"flag"
	"log"

	"github.com/boltdb/bolt"

	"github.com/skycoin/teller/src/util/dbutil"
)

//delete bucket
func deleteBucket(db *bolt.DB, bucketName string) error {
	return db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(bucketName))
	})
}

//rename bucket
func renameBucket(db *bolt.DB, bucketName, newBucketName string) error {
	newBktName := []byte(newBucketName)
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(newBktName); err != nil {
			return dbutil.NewCreateBucketFailedErr(newBktName, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		return dbutil.ForEach(tx, []byte(bucketName), func(k, v []byte) error {
			return dbutil.PutBucketValue(tx, newBktName, string(k), v)
		})
	}); err != nil {
		return err
	}

	return nil
}

func main() {
	dbname := flag.String("db", "", "db name")
	oldBkt := flag.String("oldbkt", "", "bucket name to be changed")
	newBkt := flag.String("newbkt", "", "new bucket name")
	isDeletedOld := flag.Bool("del", false, "deleted oldbkt or not, default false")
	flag.Parse()
	if *dbname == "" {
		flag.PrintDefaults()
		log.Fatal(errors.New("require db"))
	}
	if *oldBkt == "" {
		flag.PrintDefaults()
		log.Fatal(errors.New("require old bucket"))
	}
	if *newBkt == "" {
		flag.PrintDefaults()
		log.Fatal(errors.New("require new bucket"))
	}
	db, err := bolt.Open(*dbname, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Println("Failed to close db:", err)
		}
	}()

	err = renameBucket(db, *oldBkt, *newBkt)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("rename bucket %s to %s success\n", *oldBkt, *newBkt)
	}
	if *isDeletedOld {
		if err := deleteBucket(db, *oldBkt); err != nil {
			log.Fatal(err)
		} else {
			log.Printf("delete bucket %s success\n", *oldBkt)
		}
	}
}
