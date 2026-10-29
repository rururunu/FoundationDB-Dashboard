# FoundationDB Dashboard

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Русский](README.ru.md) | [日本語](README.ja.md) | [Deutsch](README.de.md) | **Français**

Console web Docker non officielle pour les clusters FoundationDB. Une image contient l’interface et le client FoundationDB **7.3.62**. La base reste hors de l’image.

FoundationDB : [github.com/apple/foundationdb](https://github.com/apple/foundationdb)  
Image : [hub.docker.com/r/runu/foundationdb-dashboard](https://hub.docker.com/r/runu/foundationdb-dashboard)

## Démarrage rapide

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  runu/foundationdb-dashboard:7.3.62
```

Si le cluster tourne déjà via Compose :

```bash
docker compose up -d
```

Ouvrez [http://127.0.0.1:8090/](http://127.0.0.1:8090/).

Publiez le port uniquement sur `127.0.0.1`. Le processus écoute `PORT` (défaut `8080`).

## Configuration

| Variable | Obligatoire | Description |
| --- | --- | --- |
| `FDB_CONNECTION_STRING` | l’un des deux | Chaîne de connexion, par exemple `docker:docker@foundationdb:4500` |
| `FDB_CLUSTER_FILE` | l’un des deux | Chemin du fichier cluster dans le conteneur. Prioritaire si les deux sont définis |
| `PORT` | non | Port d’écoute dans le conteneur (défaut `8080`) |
| `TREND_FILE` | non | Chemin du tampon de tendance lecture/écriture sur 24 h |
| `CLUSTERS_FILE` | non | Chemin du catalogue multi-clusters enregistré |
| `AUTH` | non | `on` / `off` (désactivé par défaut). Si `on`, utilisateur et mot de passe requis |
| `AUTH_USER` | avec auth | Nom d’utilisateur (64 caractères max.) |
| `AUTH_PASSWORD` | avec auth | Mot de passe (256 caractères max.) |

Exemple avec connexion :

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

## Pages

### Vue d’ensemble

![Vue d’ensemble](docs/imgs/overview.png)

Disponibilité du cluster, état des données, récupération, redondance, moteur de stockage, coordinateurs, processus et rôles, charge lecture/écriture, latence et tolérance aux pannes. Les données viennent de `\xff\xff/status/json` et se rafraîchissent environ toutes les 3 secondes tant que l’onglet est visible. Le graphique est échantillonné en arrière-plan toutes les 5 secondes et conservé environ 24 heures.

### Explorateur de données

![Explorateur de données](docs/imgs/data.png)

Arborescence à gauche, tableau des clés au centre (clé, aperçu de la valeur, type, taille), détail à droite. Le défilement charge la suite.

La recherche accepte du texte tuple, pas seulement de l’hexadécimal :

```text
"M"
("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")
```

Pris en charge : chaînes, entiers, flottants, `true` / `false` / `null`, octets `0x`, `uuid(...)`, `float32(...)`, `Versionstamp(...)` et tuples imbriqués. Un préfixe liste les clés en dessous ; une requête vide liste l’espace de clés utilisateur.

Les valeurs sont classées en JSON, texte lisible, tuple ou binaire. Les entiers little-endian de 4 et 8 octets ont une courte note. Le texte complet et l’hexadécimal repliable sont dans le panneau de détail.

Écriture :

- Créer, modifier et renommer des clés exactes
- Supprimer une clé ou plusieurs clés exactes sélectionnées (confirmation ; le serveur exige `confirm: true`)
- Import / export, sauvegarde / restauration
- Surveiller une clé et actualiser quand sa valeur change

Pas de suppression par plage.

### Configuration du cluster

![Configuration du cluster](docs/imgs/configuration.png)

Gérer les connexions enregistrées (ajouter, basculer, retirer). La connexion intégrée issue de l’environnement ne peut pas être retirée. Un basculement vide le tampon de tendance.

Pour le cluster **connecté**, vous pouvez modifier le mode de redondance, le moteur de stockage et les quatre nombres de rôles (commit proxies, GRV proxies, logs, resolvers). Coordinateurs, régions, chiffrement et mode locataire restent dans `fdbcli`.

## Build et tests

```bash
make test
```

Exécute les paquets qui n’ont pas besoin de `libfdb_c`. La construction de l’image Docker lance `go test ./...`, y compris `internal/fdbstore` (linux + cgo).

```bash
make build
make up
```

La version de l’application est inscrite à la construction de l’image (`APP_VERSION`, défaut `1.0.0`).

## Note de licence

Ce projet est une interface cliente non officielle pour FoundationDB. FoundationDB elle-même suit les licences d’Apple.
