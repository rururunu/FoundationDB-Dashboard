# FoundationDB Dashboard

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Русский](README.ru.md) | [日本語](README.ja.md) | **Deutsch** | [Français](README.fr.md)

Inoffizielle Docker-Webkonsole für FoundationDB-Cluster. Ein Image enthält die Oberfläche und den FoundationDB-Client **7.3.62**. Die Datenbank bleibt außerhalb des Images.

FoundationDB: [github.com/apple/foundationdb](https://github.com/apple/foundationdb)  
Image: [hub.docker.com/r/runu/foundationdb-dashboard](https://hub.docker.com/r/runu/foundationdb-dashboard)

## Schnellstart

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  runu/foundationdb-dashboard:7.3.62
```

Wenn der Cluster bereits per Compose läuft:

```bash
docker compose up -d
```

Öffnen Sie [http://127.0.0.1:8090/](http://127.0.0.1:8090/).

Veröffentlichen Sie den Port nur auf `127.0.0.1`. Der Prozess lauscht auf `PORT` (Standard `8080`).

## Konfiguration

| Variable | Erforderlich | Beschreibung |
| --- | --- | --- |
| `FDB_CONNECTION_STRING` | eines von beiden | Verbindungszeichenfolge, z. B. `docker:docker@foundationdb:4500` |
| `FDB_CLUSTER_FILE` | eines von beiden | Pfad zur Cluster-Datei im Container. Hat Vorrang, wenn beide gesetzt sind |
| `PORT` | nein | Port im Container (Standard `8080`) |
| `TREND_FILE` | nein | Pfad für den 24-Stunden-Lese-/Schreibtrend |
| `CLUSTERS_FILE` | nein | Pfad für den gespeicherten Multi-Cluster-Katalog |
| `AUTH` | nein | `on` / `off` (Standard aus). Bei `on` sind Benutzer und Passwort nötig |
| `AUTH_USER` | bei Auth | Anmeldename (max. 64 Zeichen) |
| `AUTH_PASSWORD` | bei Auth | Passwort (max. 256 Zeichen) |

Beispiel mit Anmeldung:

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  -e AUTH=on \
  -e AUTH_USER=admin \
  -e AUTH_PASSWORD=secret \
  runu/foundationdb-dashboard:7.3.62
```

## Seiten

### Übersicht

![Übersicht](docs/imgs/overview.png)

Verfügbarkeit, Datenzustand, Wiederherstellung, Redundanz, Speicher-Engine, Koordinatoren, Prozesse und Rollen, Lese-/Schreiblast, Latenz und Fehlertoleranz. Daten kommen aus `\xff\xff/status/json` und werden etwa alle 3 Sekunden aktualisiert, solange der Tab sichtbar ist. Das Diagramm wird im Hintergrund alle 5 Sekunden erfasst und etwa 24 Stunden gehalten.

### Datenbrowser

![Datenbrowser](docs/imgs/data.png)

Verzeichnisbaum links, Schlüsseltabelle in der Mitte (Schlüssel, Wertvorschau, Typ, Größe), Detail rechts. Beim Scrollen werden weitere Einträge geladen.

Die Suche akzeptiert Tuple-Text, nicht nur Hex:

```text
"M"
("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")
```

Unterstützt: Zeichenketten, Ganzzahlen, Gleitkommazahlen, `true` / `false` / `null`, `0x`-Bytes, `uuid(...)`, `float32(...)`, `Versionstamp(...)` und verschachtelte Tuples. Ein Präfix listet Schlüssel darunter; eine leere Suche listet den Benutzer-Schlüsselraum.

Werte werden als JSON, lesbarer Text, Tuple oder Binär eingestuft. 4- und 8-Byte-Little-Endian-Ganzzahlen erhalten einen kurzen Hinweis. Volltext und einklappbares Hex stehen im Detailbereich.

Schreiben:

- Exakte Schlüssel anlegen, bearbeiten und umbenennen
- Einen Schlüssel oder mehrere ausgewählte exakte Schlüssel löschen (Bestätigung; Server verlangt `confirm: true`)
- Import / Export, Backup / Wiederherstellung
- Einen Schlüssel beobachten und bei Wertänderung aktualisieren

Es gibt kein Bereichslöschen.

### Clusterkonfiguration

![Clusterkonfiguration](docs/imgs/configuration.png)

Gespeicherte Verbindungen verwalten (hinzufügen, wechseln, entfernen). Die eingebaute Umgebungsverbindung kann nicht entfernt werden. Ein Wechsel leert den Trendpuffer.

Für den **verbundenen** Cluster können Redundanzmodus, Speicher-Engine und die vier Rollenanzahlen (Commit-Proxies, GRV-Proxies, Logs, Resolver) geändert werden. Koordinatoren, Regionen, Verschlüsselung und Mandantenmodus bleiben bei `fdbcli`.

## Build und Tests

```bash
make test
```

Führt Pakete ohne `libfdb_c` aus. Der Docker-Image-Build führt `go test ./...` aus, einschließlich `internal/fdbstore` (linux + cgo).

```bash
make build
make up
```

Die App-Version wird beim Image-Build gesetzt (`APP_VERSION`, Standard `1.0.0`).

## Lizenz

Dieses Projekt steht unter der [MIT License](LICENSE).

Es ist eine inoffizielle Client-Oberfläche für FoundationDB. FoundationDB selbst unterliegt den Lizenzen von Apple.
