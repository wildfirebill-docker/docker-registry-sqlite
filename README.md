# Docker Registry with Multi-User Support sglite version

A modern, private Docker registry with multi-user support, featuring a beautiful Bootstrap-based web interface with multiple themes.

## Features

- **Multi-User Support**: JWT authentication with role-based access control (admin/user)
- **Modern Web UI**: Bootstrap 5 interface with 6 themes:
  - Light (default)
  - Dark
  - Ocean (blue)
  - Forest (green)
  - Sunset (warm orange)
  - Cyberpunk (neon)
- **Registry API v2**: Compatible with Docker CLI
- **Two Database Options**:
  - SQLite (lightweight, single-file)
  - MariaDB (for production/high-traffic)
- **Unraid Support**: Pre-configured XML templates
- **Standalone OS**: Can be installed as a standalone operating system
- **VM Ready**: Pre-built images for qcow2, VMDK, and VDI

## Quick Start

### Docker Compose (SQLite Version)

```bash
cd docker
docker-compose -f docker-compose.sqlite.yml up -d
```

### Docker Compose (MariaDB Version) https://github.com/wildfirebill-docker/docker-registry-mariadb

```bash
cd docker
docker-compose -f docker-compose.mariadb.yml up -d
```

### Access

- Web UI: http://localhost:8080
- Registry API: http://localhost:8080/v2/
- Default credentials: admin / admin123

## Docker Commands

### Build SQLite Version

```bash
cd docker
docker build -f Dockerfile.sqlite -t docker-registry-sqlite ..
```

### Build MariaDB Version

```bash
cd docker
docker build -f Dockerfile.mariadb -t docker-registry-mariadb ..
```

### Run SQLite Version

```bash
docker run -d \
  --name docker-registry \
  -p 8080:8080 \
  -v registry-data:/data \
  -e JWT_SECRET=your-secret \
  -e ADMIN_PASSWORD=your-password \
  docker-registry-sqlite
```

### Run MariaDB Version

```bash
docker run -d \
  --name docker-registry-mariadb \
  -p 8080:8080 \
  -e DB_SOURCE="root:password@tcp(mariadb:3306)/registry" \
  -e JWT_SECRET=your-secret \
  -e ADMIN_PASSWORD=your-password \
  --link mariadb:mariadb \
  docker-registry-mariadb
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| PORT | Server port | 8080 |
| DB_DRIVER | Database driver (sqlite/mysql/mariadb) | sqlite |
| DB_SOURCE | Full DSN connection string (overrides individual DB_* vars) | /data/registry.db |
| DB_HOST | External database host (leave empty for SQLite) | localhost |
| DB_PORT | External database port | 3306 |
| DB_USER | External database username | root |
| DB_PASSWORD | External database password | password |
| DB_NAME | External database name | registry |
| JWT_SECRET | Secret key for JWT tokens | auto-generated |
| ADMIN_USERNAME | Initial admin username | admin |
| ADMIN_PASSWORD | Initial admin password | admin123 |
| ADMIN_EMAIL | Admin email | admin@localhost |
| HTTPS_ENABLED | Enable HTTPS cookies | false |

### SQLite Usage
```bash
docker run -d \
  -p 8080:8080 \
  -v registry-data:/data \
  -e DB_DRIVER=sqlite \
  -e DB_SOURCE=/data/registry.db \
  docker-registry-sqlite
```

### External MySQL/MariaDB Usage
```bash
docker run -d \
  -p 8080:8080 \
  -e DB_DRIVER=mysql \
  -e DB_HOST=mariadb.example.com \
  -e DB_PORT=3306 \
  -e DB_USER=registry \
  -e DB_PASSWORD=secure_password \
  -e DB_NAME=registry \
  docker-registry-sqlite
```

Or use full DSN:
```bash
docker run -d \
  -p 8080:8080 \
  -e DB_DRIVER=mysql \
  -e DB_SOURCE="user:password@tcp(host:3306)/dbname" \
  docker-registry-sqlite
```

## Unraid Installation

1. Go to your Unraid web UI
2. Navigate to Docker > Add Container
3. Select "Template" and choose the XML template
4. Configure the required variables
5. Click Apply

### Unraid XML Templates

- `unraid/docker-registry-sqlite.xml` - SQLite version
- `unraid/docker-registry-mariadb.xml` - MariaDB version

## Building VM Images

### Requirements

- qemu-img (for VM image conversion)
- genisoimage or mkisofs (for ISO creation)
- Alpine Linux rootfs

### Build VM Images

```bash
cd vm-builder
./build-vm.sh
```

This creates:
- `.qcow2` - QEMU/KVM image
- `.vmdk` - VMware image
- `.vdi` - VirtualBox image

### Build Standalone ISO

```bash
cd os-builder
./build-iso.sh
```

## API Endpoints

### Authentication

- `POST /api/v1/auth/login` - Login
- `POST /api/v1/auth/register` - Register new user
- `POST /api/v1/auth/logout` - Logout
- `GET /api/v1/auth/me` - Get current user

### Repositories

- `GET /api/v1/repositories` - List repositories
- `POST /api/v1/repositories` - Create repository
- `GET /api/v1/repositories/{id}` - Get repository
- `PUT /api/v1/repositories/{id}` - Update repository
- `DELETE /api/v1/repositories/{id}` - Delete repository

### Tags

- `GET /api/v1/repositories/{id}/tags` - List tags
- `POST /api/v1/repositories/{id}/tags` - Create tag
- `DELETE /api/v1/repositories/{id}/tags/{tagId}` - Delete tag

### Admin

- `GET /api/v1/users` - List users (admin only)
- `PUT /api/v1/users/{id}` - Update user (admin only)
- `DELETE /api/v1/users/{id}` - Delete user (admin only)
- `GET /api/v1/stats` - Get statistics
- `GET /api/v1/audit` - Get audit log (admin only)

### Registry API v2

- `GET /v2/` - API version check
- `GET /v2/{name}/manifests/{reference}` - Get manifest
- `GET /v2/{name}/blobs/{digest}` - Get blob

## Docker Push/Pull

### Login to Registry

```bash
docker login localhost:8080
```

### Tag and Push Image

```bash
docker tag myimage:latest localhost:8080/myimage:latest
docker push localhost:8080/myimage:latest
```

### Pull Image

```bash
docker pull localhost:8080/myimage:latest
```

## Security Considerations

1. **Change Default Password**: Always change the default admin password
2. **JWT Secret**: Use a strong, unique JWT secret in production
3. **HTTPS**: Enable HTTPS in production environments
4. **Database**: Use MariaDB for production with multiple users
5. **Network**: Run behind a reverse proxy with SSL/TLS

## Technology Stack

- **Backend**: Go with Gorilla Mux
- **Database**: SQLite3 or MariaDB
- **Frontend**: Bootstrap 5, Vanilla JavaScript
- **Auth**: JWT + Cookie sessions
- **Security**: bcrypt password hashing

## License

MIT License
