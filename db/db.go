package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/G1itchZero/ZeroGo/utils"
	"github.com/Jeffail/gabs"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db     *sql.DB
	schema *gabs.Container
	path   string
}

func NewDB(schema *gabs.Container, p string) *DB {
	dbFile := path.Join(p, schema.S("db_file").Data().(string))

	os.MkdirAll(dbFile, 0777)
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatal(err)
	}
	return &DB{
		db:     db,
		schema: schema,
		path:   p,
	}
}

func (db *DB) Init() {
	tables := db.schema.S("tables").Data().(map[string]interface{})
	for name, t := range tables {
		table := t.(map[string]interface{})
		var cols []string
		for _, col := range table["cols"].([]interface{}) {
			c := col.([]interface{})
			cols = append(cols, fmt.Sprintf("%s %s", c[0], c[1]))
		}
		_, err := db.db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", name, strings.Join(cols, ", ")), nil)
		if err != nil {
			log.Fatal(err)
		}
		for _, index := range table["indexes"].([]interface{}) {
			db.db.Exec(index.(string), nil)
		}
	}
	maps := db.schema.S("maps").Data().(map[string]interface{})
	//TODO: match all files
	dataMap := maps["data.json"].(map[string]interface{})
	data, err := utils.LoadJSON(path.Join(db.path, "data", "data.json"))
	if err != nil {
		log.Fatal(err)
	}
	//TODO: iterate all nodes
	dataTable := (dataMap["to_table"].([]interface{}))[0].(string)
	fmt.Println(dataTable)
	for _, p := range data.S(dataTable).Data().([]interface{}) {
		post := p.(map[string]interface{})
		keys := []string{}
		ph := []string{}
		values := []interface{}{}
		for key, val := range post {
			keys = append(keys, key)
			ph = append(ph, "?")
			values = append(values, val)
		}
		q := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) values(%s)", dataTable, strings.Join(keys, ", "), strings.Join(ph, ", "))
		_, err = db.db.Exec(q, values...)
		if err != nil {
			log.Fatal(err)
		}
	}
}
