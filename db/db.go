package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

	os.MkdirAll(path.Dir(dbFile), 0777)
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

func (db *DB) creteJSONTable() {
	_, err := db.db.Exec(`
		CREATE TABLE IF NOT EXISTS json (json_id INTEGER PRIMARY KEY AUTOINCREMENT, directory VARCHAR(255), file_name VARCHAR(255))`, nil)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.db.Exec(`
		CREATE TABLE IF NOT EXISTS keyvalue (keyvalue_id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT, value INTEGER, json_id INTEGER)`, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func (db *DB) addJSON(dir string, filename string) int64 {
	name := "json"
	keys := []string{"directory", "file_name"}
	ph := []string{"?", "?"}
	q := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) values(%s)", name, strings.Join(keys, ", "), strings.Join(ph, ", "))
	result, err := db.db.Exec(q, dir, filename)
	if err != nil {
		log.Fatal(err)
	}
	index, _ := result.LastInsertId()
	// index, err := db.db.Exec("SELECT json_id from json where directory = ? and file_name = ?", dir, filename)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	return index
}

func (db *DB) createTables() {
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
}

func (db *DB) mapToTable(dataTable string, data *gabs.Container, jid int64) {
	for _, p := range data.S(dataTable).Data().([]interface{}) {
		post := p.(map[string]interface{})
		keys := []string{"json_id"}
		ph := []string{"?"}
		values := []interface{}{jid}
		for key, val := range post {
			keys = append(keys, key)
			ph = append(ph, "?")
			values = append(values, val)
		}
		q := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) values(%s)", dataTable, strings.Join(keys, ", "), strings.Join(ph, ", "))
		_, err := db.db.Exec(q, values...)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (db *DB) mapToField(dataField string, data *gabs.Container, jid int64) {
	keys := []string{"json_id", "key", "value"}
	ph := []string{"?", "?", "?"}
	q := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) values(%s)", "keyvalue", strings.Join(keys, ", "), strings.Join(ph, ", "))
	_, err := db.db.Exec(q, jid, dataField, data.S(dataField).Data())
	if err != nil {
		log.Fatal(err)
	}
}

func (db *DB) Query(q string) (interface{}, error) {
	rows, err := db.db.Query(q)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	count := len(columns)
	tableData := make([]map[string]interface{}, 0)
	values := make([]interface{}, count)
	valuePtrs := make([]interface{}, count)
	for rows.Next() {
		for i := 0; i < count; i++ {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		entry := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			entry[col] = v
		}
		tableData = append(tableData, entry)
	}
	return tableData, nil
}

func (db *DB) Init() {
	db.creteJSONTable()
	db.createTables()
	maps := db.schema.S("maps").Data().(map[string]interface{})
	for mapRe, dm := range maps {
		filepath.Walk(path.Join(db.path, "data"), func(filepath string, f os.FileInfo, err error) error {
			match, _ := regexp.MatchString(mapRe, filepath)
			if match {
				mapName := filepath
				dataMap := dm.(map[string]interface{})
				data, err := utils.LoadJSON(mapName)
				if err != nil {
					log.Fatal(err)
				}
				jid := db.addJSON("", strings.Replace(mapName, path.Join(db.path, "data")+"/", "", 1))

				dataTables := (dataMap["to_table"].([]interface{}))
				if dataTables != nil {
					for _, dt := range dataTables {
						dataTable := dt.(string)
						db.mapToTable(dataTable, data, jid)
					}
				}
				dataFields := (dataMap["to_keyvalue"].([]interface{}))
				if dataFields != nil {
					for _, dt := range dataFields {
						dataField := dt.(string)
						db.mapToField(dataField, data, jid)
					}
				}
			}
			return nil
		})
	}
}
