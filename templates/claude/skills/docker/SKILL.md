---
name: docker
description: Build and run the adb Docker image
argument-hint: "[build|run|push]"
allowed-tools: Bash, Read
---

# Docker Skill

Build and run the adb Docker image using the multi-stage Dockerfile.

## Steps

Based on the argument provided:

### "build" or no arguments
Build the Docker image:
```
docker build -t adb:latest .
```
After building, report the image size:
```
docker images adb:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
```

### "run"
Run the adb container with `--help`:
```
docker run --rm adb:latest --help
```

### "push"
Do NOT automatically push. Instead, provide instructions for the user:

1. Log in to GitHub Container Registry:
   ```
   echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
   ```

2. Tag the image:
   ```
   docker tag adb:latest ghcr.io/valter-silva-au/ai-dev-brain:latest
   ```

3. Push:
   ```
   docker push ghcr.io/valter-silva-au/ai-dev-brain:latest
   ```

### Multi-platform build (if requested)
Show the buildx command for multi-platform builds:
```
docker buildx build --platform linux/amd64,linux/arm64 -t adb:latest .
```
Note: This requires Docker Buildx to be set up.
