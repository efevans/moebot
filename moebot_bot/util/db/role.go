package db

import (
	"database/sql"
	"log"
	"strconv"
	"strings"
)

// Permission enum
type Permission int

const (
	// Default permission level, no permissions regarding what can or can't be done
	PermAll Permission = 2
	// Mod level permission, allowed to do some server changing commands
	PermMod Permission = 50
	// Used to disable something, no one can have this permission level
	PermNone Permission = 100
	// Master level permission, can't ever be ignored or disabled
	PermMaster Permission = 101
)

type Role struct {
	Id                         int
	ServerId                   int
	GroupId                    int
	RoleUid                    string
	Permission                 Permission
	ConfirmationMessage        sql.NullString
	ConfirmationSecurityAnswer sql.NullString
	Trigger                    sql.NullString
}

const (
	roleTable = `CREATE TABLE IF NOT EXISTS role(
		Id SERIAL NOT NULL PRIMARY KEY,
		ServerId INTEGER REFERENCES server(Id) ON DELETE CASCADE,
		GroupId INTEGER REFERENCES role_group(Id) ON DELETE CASCADE,
		RoleUid VARCHAR(20) NOT NULL UNIQUE,
		Permission SMALLINT NOT NULL,
		ConfirmationMessage VARCHAR CONSTRAINT role_confirmation_message_length CHECK (char_length(ConfirmationMessage) <= 1900),
		ConfirmationSecurityAnswer VARCHAR CONSTRAINT role_confirmation_security_answer_length CHECK (char_length(ConfirmationMessage) <= 1900),
		Trigger TEXT CONSTRAINT role_trigger_length CHECK(char_length(Trigger) <= 100)
	)`

	RoleMaxTriggerLength       = 100
	RoleMaxTriggerLengthString = "100"

	roleQueryServerRole  = `SELECT Id, ServerId, RoleUid, Permission, ConfirmationMessage, ConfirmationSecurityAnswer, Trigger  FROM role WHERE ServerId = $1 AND RoleUid = $2`
	roleQueryServer      = `SELECT Id, ServerId, RoleUid, Permission, ConfirmationMessage, ConfirmationSecurityAnswer, Trigger  FROM role WHERE ServerId = $1`
	roleQuery            = `SELECT Id, ServerId, RoleUid, Permission, ConfirmationMessage, ConfirmationSecurityAnswer, Trigger  FROM role WHERE Id = $1`
	roleQueryTrigger     = `SELECT Id, ServerId, RoleUid, Permission, ConfirmationMessage, ConfirmationSecurityAnswer, Trigger FROM role WHERE Trigger = $1`
	roleQueryGroup       = `SELECT Id, ServerId, RoleUid, Permission, ConfirmationMessage, ConfirmationSecurityAnswer, Trigger FROM role WHERE GroupId = $1`
	roleQueryPermissions = `SELECT Permission FROM role WHERE RoleUid = ANY ($1::varchar[])`

	roleUpdate = `UPDATE role SET Permission = $2, ConfirmationMessage = $3, ConfirmationSecurityAnswer = $4, Trigger = $5 WHERE Id = $1`

	roleInsert = `INSERT INTO role(ServerId, RoleUid, Permission, ConfirmationMessage, ConfirmationSecurityAnswer, Trigger) VALUES($1, $2, $3, $4, $5, $6) RETURNING id`

	roleDelete = `DELETE FROM role WHERE role.RoleUid = $1 AND role.ServerId = (SELECT server.id FROM server WHERE server.guilduid = $2)`
)

var (
	roleUpdateTable = []string{
		`ALTER TABLE role ADD COLUMN IF NOT EXISTS ConfirmationMessage VARCHAR`,
		`ALTER TABLE role ADD COLUMN IF NOT EXISTS ConfirmationSecurityAnswer VARCHAR`,
		`ALTER TABLE role DROP COLUMN IF EXISTS RoleType`,
		`ALTER TABLE role ADD COLUMN IF NOT EXISTS Trigger TEXT`,
		`ALTER TABLE role DROP CONSTRAINT IF EXISTS role_trigger_length`,
		`ALTER TABLE role ADD CONSTRAINT role_trigger_length CHECK(char_length(Trigger) <= 100)`,
		`ALTER TABLE role DROP CONSTRAINT IF EXISTS role_confirmation_message_length`,
		`ALTER TABLE role ADD CONSTRAINT role_confirmation_message_length CHECK(char_length(ConfirmationMessage) <= 1900)`,
		`ALTER TABLE role DROP CONSTRAINT IF EXISTS role_confirmation_security_answer_length`,
		`ALTER TABLE role ADD CONSTRAINT role_confirmation_security_answer_length CHECK(char_length(ConfirmationSecurityAnswer) <= 1900)`,
	}
)

func RoleInsertOrUpdate(role Role) error {
	row := moeDb.QueryRow(roleQueryServerRole, role.ServerId, role.RoleUid)
	var r Role
	if err := row.Scan(&r.Id, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer, &r.Trigger); err != nil {
		if err == sql.ErrNoRows {
			// no row, so insert it add in default values
			if role.Permission == -1 {
				role.Permission = PermNone
			}
			_, err = moeDb.Exec(roleInsert, role.ServerId, strings.TrimSpace(role.RoleUid), role.Permission, role.ConfirmationMessage,
				role.ConfirmationSecurityAnswer, role.Trigger)
			if err != nil {
				log.Println("Error inserting role to db")
				return err
			}
		}
	} else {
		// got a row, update it
		if role.Permission != -1 {
			r.Permission = role.Permission
		}
		if role.ConfirmationMessage.Valid {
			r.ConfirmationMessage = role.ConfirmationMessage
		}
		if role.ConfirmationSecurityAnswer.Valid {
			r.ConfirmationSecurityAnswer = role.ConfirmationSecurityAnswer
		}
		_, err = moeDb.Exec(roleUpdate, r.Id, r.Permission, r.ConfirmationMessage, r.ConfirmationSecurityAnswer, r.Trigger)
		if err != nil {
			log.Println("Error updating role to db: Id - " + strconv.Itoa(r.Id))
			return err
		}
	}
	return nil
}

func RoleQueryOrInsert(role Role) (r Role, err error) {
	row := moeDb.QueryRow(roleQueryServerRole, role.ServerId, role.RoleUid)
	if err = row.Scan(&r.Id, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer, &r.Trigger); err != nil {
		if err == sql.ErrNoRows {
			// no row, so insert it add in default values
			if role.Permission == -1 {
				role.Permission = PermNone
			}
			var insertId int
			err = moeDb.QueryRow(roleInsert, role.ServerId, strings.TrimSpace(role.RoleUid), role.Permission, role.ConfirmationMessage,
				role.ConfirmationSecurityAnswer, role.Trigger).Scan(&insertId)
			if err != nil {
				log.Println("Error inserting role to db")
				return
			}
			row := moeDb.QueryRow(roleQuery, insertId)
			if err = row.Scan(&r.Id, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer, &r.Trigger); err != nil {
				log.Println("Failed to read the newly inserted Role row. This should pretty much never happen...", err)
				return Role{}, err
			}
		}
	}
	// got a row, return it
	return
}

func RoleQueryServer(s Server) (roles []Role, err error) {
	rows, err := moeDb.Query(roleQueryServer, s.Id)
	if err != nil {
		log.Println("Error querying for role", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r Role
		if err = rows.Scan(&r.Id, &r.ServerId, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer,
			&r.Trigger); err != nil {

			log.Println("Error scanning from role table:", err)
			return
		}
		roles = append(roles, r)
	}
	return
}

func RoleQueryGroup(groupId int) (roles []Role, err error) {
	rows, err := moeDb.Query(roleQueryGroup, groupId)
	if err != nil {
		log.Println("Error querying for role", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r Role
		if err = rows.Scan(&r.Id, &r.ServerId, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer,
			&r.Trigger); err != nil {

			log.Println("Error scanning from role table:", err)
			return
		}
		roles = append(roles, r)
	}
	return
}

func RoleQueryTrigger(trigger string) (r Role, err error) {
	row := moeDb.QueryRow(roleQueryTrigger, trigger)
	err = row.Scan(&r.Id, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer, &r.Trigger)
	// return whatever we get, error or row
	return
}

func RoleQueryRoleUid(roleUid string, serverId int) (r Role, err error) {
	row := moeDb.QueryRow(roleQueryTrigger, serverId, roleUid)
	err = row.Scan(&r.Id, &r.ServerId, &r.RoleUid, &r.Permission, &r.ConfirmationMessage, &r.ConfirmationSecurityAnswer, &r.Trigger)
	// return whatever we get, error or row
	return
}

func RoleQueryPermission(roleUid []string) (p []Permission) {
	idCollection := "{" + strings.Join(roleUid, ",") + "}"
	r, err := moeDb.Query(roleQueryPermissions, idCollection)
	if err != nil {
		log.Println("Error querying for user permissions", err)
		return
	}
	for r.Next() {
		var newPerm Permission
		r.Scan(&newPerm)
		p = append(p, newPerm)
	}
	return
}

func RoleDelete(roleUid string, guildUid string) error {
	_, err := moeDb.Exec(roleDelete, roleUid, guildUid)
	if err != nil {
		log.Println("Error deleting role: ", roleUid)
	}
	return err
}

func GetPermissionFromString(s string) Permission {
	toCheck := strings.ToUpper(s)
	if toCheck == "ALL" {
		return PermAll
	} else if toCheck == "MOD" {
		return PermMod
	} else if toCheck == "NONE" {
		return PermNone
	} else if toCheck == "MASTER" {
		return PermMaster
	} else {
		return -1
	}
}

func SprintPermission(p Permission) string {
	switch p {
	case PermMod:
		return "Mod"
	case PermAll:
		return "All"
	case PermNone:
		return "None"
	case PermMaster:
		return "Master"
	default:
		return "Unknown"
	}
}

func roleCreateTable() {
	_, err := moeDb.Exec(roleTable)
	if err != nil {
		log.Println("Error creating role table", err)
		return
	}
	for _, alter := range roleUpdateTable {
		_, err = moeDb.Exec(alter)
		if err != nil {
			log.Println("Error alterting role table", err)
			return
		}
	}
}
