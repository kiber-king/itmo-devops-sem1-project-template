# Финальный проект 1 семестра

REST API сервис для загрузки и выгрузки данных о ценах.

## Требования

- Go 1.23+
- PostgreSQL 15+

## Установка и запуск

```bash
./scripts/prepare.sh
./scripts/run.sh
```

## API 

### POST /api/v0/prices

Загрузка данных из zip-архива.

```bash
curl -F "file=@sample_data.zip" http://localhost:8080/api/v0/prices
```

Ответ:
```json
{
  "total_items": 100,
  "total_categories": 15,
  "total_price": 100000
}
```

### GET /api/v0/prices

Выгрузка данных в zip-архив.

```bash
curl http://localhost:8080/api/v0/prices -o data.zip 
```

## Тестирование

```bash
./scripts/tests.sh 1
```

## База данных

- Host: localhost
- Port: 5432
- Database: project-sem-1
- User: validator
- Password: val1dat0r
