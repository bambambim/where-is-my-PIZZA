package postgres

import "database/sql"

type Repo struct {
	db *sql.DB
}

func Connect(dburl string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dburl)
	if err != nil {
		return nil, err
		//log
	}
	err = db.Ping()
	if err != nil {
		return nil, err
		//log
	}

	return db, nil
}

func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}
