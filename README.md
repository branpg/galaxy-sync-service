# Galaxy Sync Service

A Windows application to sync your GOG Galaxy library and achievements with external services.

## Features

- 📚 **Sync Library**: Reads your GOG Galaxy database and syncs your games and achievements
- ⚙️ **Windows Service**: Can run as a background service with configurable sync intervals
- 🔗 **OAuth Integration**: Connect directly with Branpg TODO or configure custom endpoints
- 🔄 **Smart Sync**: Only uploads data when changes are detected (hash-based)

## Download

Download the latest release from the [Releases page](https://github.com/branpg/galaxy-sync-service/releases).

## Usage

1. Download and run `galaxy-sync-service.exe`
2. Click **Configure** to set up your connection:
   - **Branpg TODO**: Click "Connect" to authenticate via OAuth
   - **Third-party service**: Enter your custom API URL
3. Click **Sync Now** to manually sync your library
4. (Optional) Click **Install Service** to enable automatic background sync

## Menu Options

- **Sync Now**: Immediately syncs your library (regardless of changes)
- **Install Service**: Installs as a Windows service with configurable interval
- **Uninstall Service**: Removes the Windows service
- **Configure**: Set up connection to Branpg TODO or custom endpoint

## How it Works

The application reads the GOG Galaxy SQLite database located at:
```
%ProgramData%\GOG.com\Galaxy\storage\galaxy-2.0.db
```

It extracts:
- Your owned games
- Achievement progress for each game
- Play time and last played timestamps

## Building from Source

Requires Go 1.22+ and GCC (for SQLite and Fyne).

```bash
# Install dependencies
go mod download

# Build
go build -ldflags="-H windowsgui" -o galaxy-sync-service.exe .
```

## API Format

The sync endpoint receives a POST request with the following JSON structure:

```json
{
  "games": [
    {
      "id": 1234567890,
      "title": "Game Name",
      "achievements_total": 50,
      "achievements_unlocked": 25,
      "playtime_minutes": 1234,
      "last_played": "2024-01-15T10:30:00Z"
    }
  ],
  "synced_at": "2024-01-15T12:00:00Z"
}
```

## License

MIT License
