package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var (
	db         *sql.DB
	jwtSecret  []byte
	store      *sessions.CookieStore
	dbDriver   string
	dbSource   string
)

type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Role      string    `json:"role"`
	CreatedAt string    `json:"created_at"`
	IsActive  int       `json:"is_active"`
}

type Repository struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	CreatedBy   int    `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	PullCount   int    `json:"pull_count"`
}

type Tag struct {
	ID        int    `json:"id"`
	RepoID    int    `json:"repo_id"`
	Name      string `json:"name"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func init() {
	var err error

	dbDriver = getEnv("DB_DRIVER", "sqlite")
	if dbDriver == "sqlite" {
		dbSource = getEnv("DB_SOURCE", "/data/registry.db")
	} else {
		dbHost := getEnv("DB_HOST", "localhost")
		dbPort := getEnv("DB_PORT", "3306")
		dbUser := getEnv("DB_USER", "root")
		dbPassword := getEnv("DB_PASSWORD", "password")
		dbName := getEnv("DB_NAME", "registry")
		dbSource = getEnv("DB_SOURCE", fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPassword, dbHost, dbPort, dbName))
	}

	jwtSecret = []byte(getEnv("JWT_SECRET", generateSecret()))
	store = sessions.NewCookieStore(jwtSecret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   getEnv("HTTPS_ENABLED", "false") == "true",
	}

	db, err = sql.Open(dbDriver, dbSource)
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	if err := initDB(); err != nil {
		log.Fatal(err)
	}
}

func initDB() error {
	if dbDriver == "sqlite" {
		schema := `
			CREATE TABLE IF NOT EXISTS users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT UNIQUE NOT NULL,
				email TEXT UNIQUE NOT NULL,
				password TEXT NOT NULL,
				role TEXT DEFAULT 'user',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				is_active BOOLEAN DEFAULT 1
			);
			CREATE TABLE IF NOT EXISTS repositories (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				namespace TEXT NOT NULL,
				description TEXT,
				is_private BOOLEAN DEFAULT 0,
				created_by INTEGER,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				pull_count INTEGER DEFAULT 0,
				FOREIGN KEY (created_by) REFERENCES users(id)
			);
			CREATE TABLE IF NOT EXISTS tags (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				repo_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				digest TEXT NOT NULL,
				size INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (repo_id) REFERENCES repositories(id)
			);
			CREATE TABLE IF NOT EXISTS user_repos (
				user_id INTEGER NOT NULL,
				repo_id INTEGER NOT NULL,
				permission TEXT DEFAULT 'read',
				PRIMARY KEY (user_id, repo_id),
				FOREIGN KEY (user_id) REFERENCES users(id),
				FOREIGN KEY (repo_id) REFERENCES repositories(id)
			);
			CREATE TABLE IF NOT EXISTS audit_log (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER,
				action TEXT NOT NULL,
				resource TEXT,
				ip_address TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_repo_name_namespace ON repositories(name, namespace);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_tag_repo_name ON tags(repo_id, name);
		`
		_, err := db.Exec(schema)
		if err != nil {
			return err
		}
	} else {
		schema := []string{
			`CREATE TABLE IF NOT EXISTS users (
				id INT AUTO_INCREMENT PRIMARY KEY,
				username VARCHAR(255) UNIQUE NOT NULL,
				email VARCHAR(255) UNIQUE NOT NULL,
				password VARCHAR(255) NOT NULL,
				role VARCHAR(50) DEFAULT 'user',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				is_active TINYINT(1) DEFAULT 1
			)`,
			`CREATE TABLE IF NOT EXISTS repositories (
				id INT AUTO_INCREMENT PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				namespace VARCHAR(255) NOT NULL,
				description TEXT,
				is_private TINYINT(1) DEFAULT 0,
				created_by INT,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				pull_count INT DEFAULT 0,
				FOREIGN KEY (created_by) REFERENCES users(id),
				UNIQUE KEY unique_repo (name, namespace)
			)`,
			`CREATE TABLE IF NOT EXISTS tags (
				id INT AUTO_INCREMENT PRIMARY KEY,
				repo_id INT NOT NULL,
				name VARCHAR(255) NOT NULL,
				digest VARCHAR(255) NOT NULL,
				size BIGINT DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (repo_id) REFERENCES repositories(id),
				UNIQUE KEY unique_tag (repo_id, name)
			)`,
			`CREATE TABLE IF NOT EXISTS user_repos (
				user_id INT NOT NULL,
				repo_id INT NOT NULL,
				permission VARCHAR(50) DEFAULT 'read',
				PRIMARY KEY (user_id, repo_id),
				FOREIGN KEY (user_id) REFERENCES users(id),
				FOREIGN KEY (repo_id) REFERENCES repositories(id)
			)`,
			`CREATE TABLE IF NOT EXISTS audit_log (
				id INT AUTO_INCREMENT PRIMARY KEY,
				user_id INT,
				action VARCHAR(255) NOT NULL,
				resource VARCHAR(255),
				ip_address VARCHAR(45),
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)`,
		}

		for _, stmt := range schema {
			_, err := db.Exec(stmt)
			if err != nil {
				return err
			}
		}
	}

	var count int
	var err error
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(getEnv("ADMIN_PASSWORD", "admin123")), bcrypt.DefaultCost)
		_, err = db.Exec(
			"INSERT INTO users (username, email, password, role) VALUES (?, ?, ?, 'admin')",
			getEnv("ADMIN_USERNAME", "admin"),
			getEnv("ADMIN_EMAIL", "admin@localhost"),
			string(hashedPassword),
		)
		if err != nil {
			return err
		}
		log.Println("Default admin user created")
	}

	return nil
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/api/v1/health", healthHandler).Methods("GET")
	r.HandleFunc("/api/v1/auth/login", loginHandler).Methods("POST")
	r.HandleFunc("/api/v1/auth/register", registerHandler).Methods("POST")
	r.HandleFunc("/api/v1/auth/logout", logoutHandler).Methods("POST")
	r.HandleFunc("/api/v1/auth/me", authMiddleware(getMeHandler)).Methods("GET")

	r.HandleFunc("/api/v1/users", authMiddleware(adminMiddleware(getUsersHandler))).Methods("GET")
	r.HandleFunc("/api/v1/users/{id}", authMiddleware(adminMiddleware(updateUserHandler))).Methods("PUT")
	r.HandleFunc("/api/v1/users/{id}", authMiddleware(adminMiddleware(deleteUserHandler))).Methods("DELETE")

	r.HandleFunc("/api/v1/repositories", authMiddleware(getRepositoriesHandler)).Methods("GET")
	r.HandleFunc("/api/v1/repositories", authMiddleware(createRepositoryHandler)).Methods("POST")
	r.HandleFunc("/api/v1/repositories/{id}", authMiddleware(getRepositoryHandler)).Methods("GET")
	r.HandleFunc("/api/v1/repositories/{id}", authMiddleware(updateRepositoryHandler)).Methods("PUT")
	r.HandleFunc("/api/v1/repositories/{id}", authMiddleware(deleteRepositoryHandler)).Methods("DELETE")

	r.HandleFunc("/api/v1/repositories/{id}/tags", authMiddleware(getTagsHandler)).Methods("GET")
	r.HandleFunc("/api/v1/repositories/{id}/tags", authMiddleware(createTagHandler)).Methods("POST")
	r.HandleFunc("/api/v1/repositories/{id}/tags/{tagId}", authMiddleware(deleteTagHandler)).Methods("DELETE")

	r.HandleFunc("/api/v1/stats", authMiddleware(getStatsHandler)).Methods("GET")
	r.HandleFunc("/api/v1/audit", authMiddleware(adminMiddleware(getAuditLogHandler))).Methods("GET")

	r.HandleFunc("/v2/", registryV2Handler).Methods("GET", "HEAD")
	r.HandleFunc("/v2/{name:.+}/manifests/{reference}", registryManifestHandler).Methods("GET", "PUT", "HEAD")
	r.HandleFunc("/v2/{name:.+}/blobs/{digest}", registryBlobHandler).Methods("GET", "HEAD")
	r.HandleFunc("/v2/{name:.+}/blobs/uploads/", registryUploadHandler).Methods("POST")
	r.HandleFunc("/v2/{name:.+}/blobs/uploads/{uuid}", registryUploadHandler).Methods("PATCH", "PUT", "DELETE")

	r.HandleFunc("/api/v1/themes", getThemesHandler).Methods("GET")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("/app/static"))))
	r.HandleFunc("/", indexHandler).Methods("GET")

	port := getEnv("PORT", "8080")
	log.Printf("Starting server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "healthy",
		Data: map[string]interface{}{
			"version":   "1.0.0",
			"db_driver": dbDriver,
			"uptime":    time.Since(time.Now()).String(),
		},
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var user User
	err := db.QueryRow(
		"SELECT id, username, email, password, role, created_at, is_active FROM users WHERE username = ? OR email = ?",
		req.Username, req.Username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.Role, &user.CreatedAt, &user.IsActive)

	if err != nil {
		sendError(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if user.IsActive == 0 {
		sendError(w, "Account is disabled", http.StatusForbidden)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		sendError(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		sendError(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["user_id"] = user.ID
	session.Values["username"] = user.Username
	session.Values["role"] = user.Role
	session.Save(r, w)

	logAudit(user.ID, "login", "", getClientIP(r))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"token":    tokenString,
			"user":     user,
			"expires":  time.Now().Add(time.Hour * 24).Format(time.RFC3339),
		},
	})
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		sendError(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		sendError(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	result, err := db.Exec(
		"INSERT INTO users (username, email, password) VALUES (?, ?, ?)",
		req.Username, req.Email, string(hashedPassword),
	)
	if err != nil {
		sendError(w, "Username or email already exists", http.StatusConflict)
		return
	}

	id, _ := result.LastInsertId()
	logAudit(int(id), "register", "", getClientIP(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "User registered successfully",
	})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	session.Options.MaxAge = -1
	session.Save(r, w)

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Logged out successfully",
	})
}

func getMeHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	var user User
	err := db.QueryRow(
		"SELECT id, username, email, role, created_at, is_active FROM users WHERE id = ?",
		userID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Role, &user.CreatedAt, &user.IsActive)

	if err != nil {
		sendError(w, "User not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    user,
	})
}

func getUsersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, username, email, role, created_at, is_active FROM users ORDER BY created_at DESC")
	if err != nil {
		sendError(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.CreatedAt, &u.IsActive); err != nil {
			sendError(w, "Failed to parse users", http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    users,
	})
}

func updateUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var updates struct {
		Role     *string `json:"role"`
		IsActive *bool   `json:"is_active"`
		Email    *string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		sendError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	query := "UPDATE users SET "
	args := []interface{}{}
	i := 0

	if updates.Role != nil {
		query += fmt.Sprintf("role = $%d", i+1)
		args = append(args, *updates.Role)
		i++
	}
	if updates.IsActive != nil {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("is_active = $%d", i+1)
		args = append(args, *updates.IsActive)
		i++
	}
	if updates.Email != nil {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("email = $%d", i+1)
		args = append(args, *updates.Email)
		i++
	}

	if i == 0 {
		sendError(w, "No fields to update", http.StatusBadRequest)
		return
	}

	if dbDriver == "sqlite" {
		query = strings.ReplaceAll(query, "$", "?")
	}

	query += fmt.Sprintf(" WHERE id = $%d", i+1)
	if dbDriver == "sqlite" {
		query = strings.ReplaceAll(query, "$", "?")
	}
	args = append(args, id)

	_, err := db.Exec(query, args...)
	if err != nil {
		sendError(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	logAudit(getUserID(r), "update_user", fmt.Sprintf("user_id:%s", id), getClientIP(r))

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "User updated successfully",
	})
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	userID := getUserID(r)
	if fmt.Sprintf("%d", userID) == id {
		sendError(w, "Cannot delete yourself", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		sendError(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	logAudit(userID, "delete_user", fmt.Sprintf("user_id:%s", id), getClientIP(r))

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "User deleted successfully",
	})
}

func getRepositoriesHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	userRole := getRole(r)

	query := `
		SELECT r.id, r.name, r.namespace, r.description, r.is_private, 
			   r.created_by, r.created_at, r.updated_at, r.pull_count
		FROM repositories r
	`

	if userRole != "admin" {
		query += `
			LEFT JOIN user_repos ur ON r.id = ur.repo_id AND ur.user_id = ?
			WHERE r.is_private = 0 OR ur.user_id IS NOT NULL OR r.created_by = ?
		`
	}

	query += " ORDER BY r.updated_at DESC"

	var rows *sql.Rows
	var err error

	if userRole != "admin" {
		rows, err = db.Query(query, userID, userID)
	} else {
		rows, err = db.Query(query)
	}

	if err != nil {
		sendError(w, "Failed to fetch repositories", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var repos []Repository
	for rows.Next() {
		var repo Repository
		if err := rows.Scan(&repo.ID, &repo.Name, &repo.Namespace, &repo.Description,
			&repo.IsPrivate, &repo.CreatedBy, &repo.CreatedAt, &repo.UpdatedAt, &repo.PullCount); err != nil {
			sendError(w, "Failed to parse repositories", http.StatusInternalServerError)
			return
		}
		repos = append(repos, repo)
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    repos,
	})
}

func createRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var repo Repository
	if err := json.NewDecoder(r.Body).Decode(&repo); err != nil {
		sendError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(
		"INSERT INTO repositories (name, namespace, description, is_private, created_by) VALUES (?, ?, ?, ?, ?)",
		repo.Name, repo.Namespace, repo.Description, repo.IsPrivate, userID,
	)
	if err != nil {
		sendError(w, "Failed to create repository", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	repo.ID = int(id)
	repo.CreatedBy = userID

	logAudit(userID, "create_repository", fmt.Sprintf("repo_id:%d", id), getClientIP(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    repo,
	})
}

func getRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var repo Repository
	err := db.QueryRow(
		`SELECT id, name, namespace, description, is_private, created_by, created_at, updated_at, pull_count 
		 FROM repositories WHERE id = ?`,
		id,
	).Scan(&repo.ID, &repo.Name, &repo.Namespace, &repo.Description, &repo.IsPrivate,
		&repo.CreatedBy, &repo.CreatedAt, &repo.UpdatedAt, &repo.PullCount)

	if err != nil {
		sendError(w, "Repository not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    repo,
	})
}

func updateRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var updates struct {
		Description *string `json:"description"`
		IsPrivate   *bool   `json:"is_private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		sendError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	_, err := db.Exec(
		"UPDATE repositories SET description = COALESCE(?, description), is_private = COALESCE(?, is_private) WHERE id = ?",
		updates.Description, updates.IsPrivate, id,
	)
	if err != nil {
		sendError(w, "Failed to update repository", http.StatusInternalServerError)
		return
	}

	logAudit(getUserID(r), "update_repository", fmt.Sprintf("repo_id:%s", id), getClientIP(r))

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Repository updated successfully",
	})
}

func deleteRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	_, err := db.Exec("DELETE FROM repositories WHERE id = ?", id)
	if err != nil {
		sendError(w, "Failed to delete repository", http.StatusInternalServerError)
		return
	}

	logAudit(getUserID(r), "delete_repository", fmt.Sprintf("repo_id:%s", id), getClientIP(r))

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Repository deleted successfully",
	})
}

func getTagsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]

	rows, err := db.Query(
		"SELECT id, repo_id, name, digest, size, created_at FROM tags WHERE repo_id = ? ORDER BY created_at DESC",
		repoID,
	)
	if err != nil {
		sendError(w, "Failed to fetch tags", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.RepoID, &t.Name, &t.Digest, &t.Size, &t.CreatedAt); err != nil {
			sendError(w, "Failed to parse tags", http.StatusInternalServerError)
			return
		}
		tags = append(tags, t)
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    tags,
	})
}

func createTagHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]

	var tag Tag
	if err := json.NewDecoder(r.Body).Decode(&tag); err != nil {
		sendError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(
		"INSERT INTO tags (repo_id, name, digest, size) VALUES (?, ?, ?, ?)",
		repoID, tag.Name, tag.Digest, tag.Size,
	)
	if err != nil {
		sendError(w, "Failed to create tag", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	tag.ID = int(id)

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    tag,
	})
}

func deleteTagHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tagID := vars["tagId"]

	_, err := db.Exec("DELETE FROM tags WHERE id = ?", tagID)
	if err != nil {
		sendError(w, "Failed to delete tag", http.StatusInternalServerError)
		return
	}

	logAudit(getUserID(r), "delete_tag", fmt.Sprintf("tag_id:%s", tagID), getClientIP(r))

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Tag deleted successfully",
	})
}

func getStatsHandler(w http.ResponseWriter, r *http.Request) {
	var userCount, repoCount, tagCount int
	var totalSize int64

	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	db.QueryRow("SELECT COUNT(*) FROM repositories").Scan(&repoCount)
	db.QueryRow("SELECT COUNT(*) FROM tags").Scan(&tagCount)
	db.QueryRow("SELECT COALESCE(SUM(size), 0) FROM tags").Scan(&totalSize)

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"users":       userCount,
			"repositories": repoCount,
			"tags":        tagCount,
			"total_size":  totalSize,
		},
	})
}

func getAuditLogHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT al.id, al.user_id, u.username, al.action, al.resource, al.ip_address, al.created_at
		FROM audit_log al
		LEFT JOIN users u ON al.user_id = u.id
		ORDER BY al.created_at DESC
		LIMIT 100
	`)
	if err != nil {
		sendError(w, "Failed to fetch audit log", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AuditEntry struct {
		ID        int    `json:"id"`
		UserID    int    `json:"user_id"`
		Username  string `json:"username"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		IPAddress string `json:"ip_address"`
		CreatedAt string `json:"created_at"`
	}

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.Resource, &e.IPAddress, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    entries,
	})
}

func getThemesHandler(w http.ResponseWriter, r *http.Request) {
	themes := []map[string]interface{}{
		{
			"id":          "light",
			"name":        "Light",
			"description": "Clean light theme",
			"is_default":  true,
		},
		{
			"id":          "dark",
			"name":        "Dark",
			"description": "Dark theme for low-light environments",
			"is_default":  false,
		},
		{
			"id":          "ocean",
			"name":        "Ocean",
			"description": "Blue ocean-inspired theme",
			"is_default":  false,
		},
		{
			"id":          "forest",
			"name":        "Forest",
			"description": "Green nature-inspired theme",
			"is_default":  false,
		},
		{
			"id":          "sunset",
			"name":        "Sunset",
			"description": "Warm sunset colors",
			"is_default":  false,
		},
		{
			"id":          "cyberpunk",
			"name":        "Cyberpunk",
			"description": "Neon cyberpunk theme",
			"is_default":  false,
		},
	}

	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    themes,
	})
}

func registryV2Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
}

func registryManifestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
}

func registryBlobHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
}

func registryUploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	w.WriteHeader(http.StatusAccepted)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "/app/static/index.html")
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := store.Get(r, "session")
		if err != nil {
			sendUnauthorized(w)
			return
		}

		userID, ok := session.Values["user_id"].(int)
		if !ok {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				sendUnauthorized(w)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				return jwtSecret, nil
			})

			if err != nil || !token.Valid {
				sendUnauthorized(w)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				sendUnauthorized(w)
				return
			}

			userID = int(claims["user_id"].(float64))
		}

		ctx := r.Context()
		ctx = contextWithUserID(ctx, userID)
		ctx = contextWithRole(ctx, session.Values["role"].(string))
		next(w, r.WithContext(ctx))
	}
}

func adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := getRole(r)
		if role != "admin" {
			sendError(w, "Admin access required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func contextWithUserID(ctx context.Context, userID int) context.Context {
	return context.WithValue(ctx, "user_id", userID)
}

func contextWithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, "role", role)
}

func getUserID(r *http.Request) int {
	if val := r.Context().Value("user_id"); val != nil {
		return val.(int)
	}
	return 0
}

func getRole(r *http.Request) string {
	if val := r.Context().Value("role"); val != nil {
		return val.(string)
	}
	return ""
}

func sendError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Message: message,
	})
}

func sendUnauthorized(w http.ResponseWriter) {
	sendError(w, "Unauthorized", http.StatusUnauthorized)
}

func logAudit(userID int, action, resource, ip string) {
	db.Exec(
		"INSERT INTO audit_log (user_id, action, resource, ip_address) VALUES (?, ?, ?, ?)",
		userID, action, resource, ip,
	)
}

func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
