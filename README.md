# 🚀 WhatsApp Render Bridge

Un bridge API REST para WhatsApp optimizado para deployment en Render.com

## ✨ Características

- 🌐 **API REST universal** - Compatible con cualquier aplicación
- 📱 **QR Code en navegador** - Sin necesidad de terminal local
- 🚀 **Deploy automático** - GitHub → Render sin configuración
- 💰 **Costo-efectivo** - Solo $7/mes en Render Starter
- 🔄 **Auto-healing** - Re-conecta automáticamente si pierde sesión

## 📋 Endpoints Disponibles

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/` | Página principal con estado del servicio |
| `GET` | `/api/qr` | Ver código QR para autenticación |
| `GET` | `/api/status` | Estado del servicio (JSON) |
| `POST` | `/api/send` | Enviar mensajes WhatsApp |

## 🚀 Deploy en Render

### Paso 1: Preparar repositorio
```bash
# Clonar o crear repositorio
git clone https://github.com/tu-usuario/whatsapp-render.git
cd whatsapp-render

# Copiar archivos del proyecto
cp -r whatsapp-render/* .

# Commit inicial
git add .
git commit -m "Initial WhatsApp Render Bridge"
git push origin main
```

### Paso 2: Crear servicio en Render
1. Ve a [render.com](https://render.com) y crea cuenta
2. **New** → **Web Service**
3. Conecta tu repositorio GitHub
4. Render detectará automáticamente `render.yaml`
5. Click **Deploy** 

### Paso 3: Primera autenticación
1. Una vez desplegado, ve a `https://tu-app.onrender.com`
2. Click **📱 QR Code** o ve a `/api/qr`
3. Escanea el QR con WhatsApp móvil:
   - WhatsApp → Menú ⋮ → **WhatsApp Web**
   - **Escanear código QR**
4. ¡Listo! El servicio queda autenticado ~20 días

## 📱 Uso de la API

### Enviar mensaje de texto
```bash
curl -X POST https://tu-app.onrender.com/api/send \
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

### ❌ "Build failed"
```bash
# Verificar que render.yaml esté en la raíz
# Verificar go.mod tiene las dependencias correctas
```

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