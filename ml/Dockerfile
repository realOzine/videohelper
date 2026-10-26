FROM ghcr.io/astral-sh/uv:0.12.23 AS uv
FROM python:3.12-slim
COPY --from=uv /uv /uvx /bin/
# Build context is the repository root; profiles install regular wheels, never editable imports.
ARG ML_PROFILE=speech-audio-cpu
COPY ml /srv/ml
WORKDIR /srv/ml/profiles/${ML_PROFILE}
RUN uv sync --locked --no-dev --no-editable
ENV PATH="/srv/ml/profiles/${ML_PROFILE}/.venv/bin:$PATH" \
    ML_FAKE=0 ML_CACHE_DIR=/cache/media HF_HOME=/cache/huggingface
EXPOSE 8000
CMD ["videohelper-ml", "serve", "--models", "models.yaml", "--host", "0.0.0.0", "--port", "8000"]
