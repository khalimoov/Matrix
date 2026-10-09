package store

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type Role string

const (
	RoleAdmin    Role = "admin"    // full control: devices, users, settings
	RoleOperator Role = "operator" // live view, playback, PTZ, events
	RoleViewer   Role = "viewer"   // live view only
)

func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleOperator || r == RoleViewer
}

// AtLeast reports whether r grants every permission that min grants.
func (r Role) AtLeast(min Role) bool {
	rank := map[Role]int{RoleViewer: 1, RoleOperator: 2, RoleAdmin: 3}
	return rank[r] >= rank[min]
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	PasswordHash string    `json:"-"`
}

const userCols = `id, username, password_hash, role, created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var created int64
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &created)
	u.CreatedAt = time.Unix(created, 0)
	return u, notFound(err)
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) UserByID(id int64) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) UserByName(name string) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE username = ?`, strings.TrimSpace(name)))
}

func (s *Store) CreateUser(username, passwordHash string, role Role) (User, error) {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		strings.TrimSpace(username), passwordHash, role, now.Unix())
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username, Role: role, CreatedAt: now, PasswordHash: passwordHash}, nil
}

func (s *Store) UpdateUserRole(id int64, role Role) error {
	return s.execOne(`UPDATE users SET role = ? WHERE id = ?`, role, id)
}

// UpdateUserPassword changes the password and signs the user out everywhere.
func (s *Store) UpdateUserPassword(id int64, passwordHash string) error {
	if err := s.execOne(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	return err
}

func (s *Store) DeleteUser(id int64) error {
	return s.execOne(`DELETE FROM users WHERE id = ?`, id)
}

func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = ?`, RoleAdmin).Scan(&n)
	return n, err
}

// Sessions are stored as SHA-256 hashes so a leaked database does not leak live tokens.

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSession(token string, userID int64, ttl time.Duration) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, time.Now().Add(ttl).Unix())
	return err
}

func (s *Store) SessionUser(token string) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT u.id, u.username, u.password_hash, u.role, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(token), time.Now().Unix()))
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

func (s *Store) DeleteExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	return err
}

func (s *Store) execOne(query string, args ...any) error {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
