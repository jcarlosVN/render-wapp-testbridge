# 🚀 WhatsApp Render Bridge

Un bridge API REST para WhatsApp optimizado para deployment en Render.com

## ✨ Características

- 🌐 **API REST universal** - Compatible con cualquier aplicación
- 📱 **QR Code en navegador** - Sin necesidad de terminal local
- 🚀 **Deploy automático** - GitHub → Render sin configuración
- 💰 **Costo-efectivo** - Solo $7/mes en Render Starter
- 🔄 **Auto-healing** - Re-conecta automáticamente si pierde sesión
- ⚡ **Sin downtime** - Limpieza de base de datos sin reiniciar servicio

## 📋 Endpoints Disponibles

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/` | Página principal con estado del servicio |
| `GET` | `/api/qr` | Ver código QR para autenticación |
| `GET` | `/api/status` | Estado del servicio (JSON) |
| `POST` | `/api/send` | Enviar mensajes WhatsApp |
| `POST` | `/api/reauth` | Forzar nueva autenticación |
| `POST` | `/api/clean` | Limpiar base de datos corrupta (⚡ sin downtime) |

## 🚀 Deploy en Render

### Paso 1: Preparar repositorio

⚠️ **CRÍTICO**: Debe incluir `go.sum` en el repositorio

```bash
# Clonar o crear repositorio
git clone https://github.com/tu-usuario/whatsapp-render.git
cd whatsapp-render

# Copiar archivos del proyecto
cp -r whatsapp-render/* .

# IMPORTANTE: Generar go.sum localmente antes de subir
go mod download
go mod tidy

# Verificar que go.sum fue creado
ls -la go.sum

# Commit inicial (DEBE incluir go.sum)
git add .
git commit -m "Initial WhatsApp Render Bridge"
git push origin main
```

**📝 Nota**: Si no incluyes `go.sum`, Render fallará con este error:
```
missing go.sum entry for module providing package google.golang.org/protobuf/proto; to add:
go mod download google.golang.org/protobuf
==> Build failed 😞
```

### Paso 2: Crear servicio en Render

⚠️ **Importante**: Render puede no leer automáticamente el `render.yaml`. Si esto ocurre, configura manualmente:

1. Ve a [render.com](https://render.com) y crea cuenta
2. **New** → **Web Service**
3. Conecta tu repositorio GitHub
4. **Si Render detecta automáticamente `render.yaml`**: Click **Deploy**
5. **Si NO detecta el .yaml (configuración manual)** - Usa estos valores exactos:
   - **Language**: Go ###aparece solo
   - **Branch**: main ###aparece solo
   - **Build Command**: `go mod download && go build -o main main.go` ###cambiar como dice aquí
   - **Start Command**: `./main` ###cambiar como dice aquí
   - **Environment Variables**: 
     - `QR_TOKEN`: Genera un token seguro (ej: `abcd1234efgh5678`)
   - Click **Deploy**

### 🔒 Token de Seguridad QR

El sistema incluye protección por token para el endpoint `/api/qr`:
- **Automático**: `render.yaml` genera un token seguro automáticamente
- **Manual**: Si configuras manualmente, agrega variable `QR_TOKEN` con un valor aleatorio
- **Sin token**: Si no defines `QR_TOKEN`, el QR será público (no recomendado) 

### Paso 3: Primera autenticación
1. Una vez desplegado, ve a `https://tu-app.onrender.com`
2. Click **📱 QR Code** (incluye automáticamente el token de seguridad)
3. **Alternativa directa**: Ve a `/api/qr?token=TU_TOKEN` (reemplaza con tu token)
3. Escanea el QR con WhatsApp móvil:
   - WhatsApp → Menú ⋮ → **WhatsApp Web**
   - **Escanear código QR**
4. ¡Listo! El servicio queda autenticado ~20 días

## 📱 Uso de la API

### Enviar mensaje de texto
```bash
curl -X POST https://render-wapp-testbridge.onrender.com/api/send \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "51959812636",
    "message": "¡Hola desde Render!"
  }'
```

### Enviar archivo multimedia
```bash
curl -X POST https://tu-app.onrender.com/api/send \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "51959812636@s.whatsapp.net",
    "message": "Mira esta imagen",
    "media_path": "/ruta/absoluta/imagen.jpg"
  }'
```

### Enviar a grupo
```bash
curl -X POST https://tu-app.onrender.com/api/send \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "grupo-id@g.us",
    "message": "Mensaje al grupo"
  }'
```

### Verificar estado
```bash
curl https://tu-app.onrender.com/api/status
```

### Forzar nueva autenticación
```bash
curl -X POST https://tu-app.onrender.com/api/reauth
```

### Limpiar base de datos corrupta (⚡ Sin downtime)
```bash
curl -X POST https://tu-app.onrender.com/api/clean
```
**Respuesta**:
```json
{
  "success": true,
  "message": "Database cleaned successfully. New QR code will be available shortly at /api/qr"
}
```

**Respuesta ejemplo:**
```json
{
  "connected": true,
  "needs_qr": false,
  "has_qr": false,
  "uptime": "2h30m15s",
  "qr_url": "https://tu-app.onrender.com/api/qr",
  "service": "whatsapp-render-bridge",
  "version": "1.0.0",
  "timestamp": 1735834567
}
```

## 🔧 Formatos de destinatario

| Tipo | Formato | Ejemplo |
|------|---------|---------|
| **Número telefónico** | `país + número` (sin +) | `51959812636` |
| **JID individual** | `número@s.whatsapp.net` | `51959812636@s.whatsapp.net` |
| **JID grupo** | `id-grupo@g.us` | `123456789@g.us` |

## 🔄 Re-autenticación (cada ~20 días)

Cuando expire la sesión:

1. **Los logs mostrarán**: `"Device logged out, QR scan needed"`
2. **Ve a**: `https://tu-app.onrender.com/api/qr`
3. **Escanea** el nuevo QR code
4. **Funciona** otros ~20 días automáticamente

## ⚡ Limpieza de Base de Datos (Sin Downtime)

Si encuentras errores de base de datos corrupta:

### 🖱️ **Método Web** (Recomendado):
1. Ve a `https://tu-app.onrender.com`
2. Click **🧹 Limpiar base de datos**
3. Confirma la acción
4. **Automático**: Redirige al nuevo QR en 2 segundos
5. Escanea y listo ✅

### 🔧 **Método API**:
```bash
curl -X POST https://tu-app.onrender.com/api/clean
# Respuesta inmediata, nuevo QR disponible en ~2-3 segundos
```

**⚡ Ventajas**: 
- Sin reinicio del servicio
- Sin downtime (0 segundos offline)
- Proceso automático de 2-3 segundos
- Redirección automática al QR

## 🧪 Prueba local

```bash
# Ejecutar localmente
go mod download
go run main.go

# Acceder
open http://localhost:8080
```

## 📂 Estructura del proyecto

```
whatsapp-render/
├── main.go          # Aplicación principal Go
├── go.mod           # Dependencias Go
├── render.yaml      # Configuración Render
├── README.md        # Este archivo
└── store/           # Base de datos sesión (auto-creada)
    └── whatsapp.db
```

## 💰 Costos Render

- **Starter Plan**: $7/mes
- **Uptime**: 24/7 sin sleep
- **Ancho de banda**: 100GB/mes
- **Deploy**: Ilimitados desde GitHub

## 🐛 Solución de problemas

### ❌ "Build failed" - Missing go.sum
**Error completo**:
```
main.go:27:2: missing go.sum entry for module providing package google.golang.org/protobuf/proto; to add:
go mod download google.golang.org/protobuf
==> Build failed 😞
```

**Solución**:
```bash
# En tu máquina local:
cd tu-proyecto
go mod download
go mod tidy
git add go.sum
git commit -m "Add missing go.sum file"
git push origin main
```

### ❌ "Build failed" - render.yaml no detectado
**Síntomas**: Render no detecta configuración automática

**Solución**: Configuración manual en Render:
- **Language**: Go
- **Build Command**: `go mod download && go build -o main main.go`
- **Start Command**: `./main`

### ❌ "FOREIGN KEY constraint failed" 
**Error**: `Failed to pair device: failed to store main device identity: FOREIGN KEY constraint failed`

**Solución automática**: El código detecta y limpia automáticamente
**Solución manual** (⚡ Sin downtime - 2-3 segundos):
1. Ve a tu app: `https://tu-app.onrender.com`
2. Click **🧹 Limpiar base de datos**
3. **Automático**: Te redirige al QR en 2 segundos
4. Escanea el nuevo código QR

### ❌ "Service unhealthy"
- Ve a `/api/status` para ver el estado
- Revisa logs en Render dashboard
- Puede necesitar re-autenticación QR

### ❌ "Not connected to WhatsApp"
- Ve a `/api/qr` para escanear código
- Verifica que WhatsApp móvil tenga internet
- La sesión expira cada ~20 días

### ❌ "Error parsing JID"
- Verifica formato del destinatario
- Usar número sin signos: `51959812636`
- Para grupos usar JID completo: `grupo@g.us`

## 🔒 Seguridad

- ✅ Sin credenciales expuestas (usa sesión WhatsApp)
- ✅ Base de datos local encriptada
- ✅ HTTPS automático en Render
- ⚠️ API sin autenticación (agregar si necesario)

## 📞 Soporte

- **GitHub Issues**: Para bugs y mejoras
- **Render Docs**: [docs.render.com](https://docs.render.com)
- **WhatsApp Web API**: Usa whatsmeow library

---

## 🎉 ¡Todo listo!

Tu WhatsApp Bridge está corriendo 24/7 en Render. Solo necesitas:

1. **Deploy** una vez
2. **Escanear QR** una vez cada ~20 días  
3. **Usar API** las veces que quieras

**URL de tu servicio**: `https://tu-app-nombre.onrender.com`