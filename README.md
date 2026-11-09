# DEUS coding challenge - RabbitMQ helper

1. First, run the RabbitMQ container with:
```bash
docker compose up -d
```

This command will start a RabbitMQ container with the management interface enabled, accessible at `http://localhost:15672` (username: `deus_user`, password: `deus_pass`).


## Important Notes
- Remember that RabbitMQ requires authentication with the configured credentials
- The management interface is useful for monitoring the status of queues
- If you encounter "ACCESS_REFUSED" errors, verify that you're using the correct credentials
