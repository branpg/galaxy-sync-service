#!/bin/bash
# Script para configurar el runner de GitHub Actions para galaxy-sync-service
#
# Requisitos:
# - Docker y Docker Compose instalados
# - Token de acceso de GitHub con permisos de Administration

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Configuración del GitHub Runner para galaxy-sync-service ==="
echo ""

# Verificar si ya existe .env
if [ -f .env ]; then
    echo "⚠️  El archivo .env ya existe."
    read -p "¿Deseas sobrescribirlo? (s/N): " overwrite
    if [[ ! "$overwrite" =~ ^[Ss]$ ]]; then
        echo "Manteniendo configuración existente."
    else
        rm .env
    fi
fi

# Crear .env si no existe
if [ ! -f .env ]; then
    echo ""
    echo "Necesitas un Fine-grained Personal Access Token de GitHub."
    echo "Créalo en: https://github.com/settings/tokens?type=beta"
    echo ""
    echo "Configuración del token:"
    echo "  - Repository access: Only select repositories → branpg/galaxy-sync-service"
    echo "  - Permissions: Administration → Read and write"
    echo ""
    read -p "Pega tu ACCESS_TOKEN: " token
    
    if [ -z "$token" ]; then
        echo "❌ Token vacío. Abortando."
        exit 1
    fi
    
    echo "ACCESS_TOKEN=$token" > .env
    chmod 600 .env
    echo "✅ Archivo .env creado"
fi

echo ""
echo "Iniciando el runner..."
docker compose up -d

echo ""
echo "=== Runner iniciado ==="
echo ""
echo "Verifica el estado con: docker logs -f github-runner-galaxy-sync"
echo "El runner debería aparecer en: https://github.com/branpg/galaxy-sync-service/settings/actions/runners"
