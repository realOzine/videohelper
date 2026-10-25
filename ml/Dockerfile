FROM ghcr.io/astral-sh/uv:0.8.22 AS uv
FROM python:3.12-slim
COPY --from=uv /uv /uvx /bin/
WORKDIR /srv/ml
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev
COPY app ./app
COPY models.yaml ./models.yaml
ENV PATH="/srv/ml/.venv/bin:$PATH" ML_FAKE=1
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
