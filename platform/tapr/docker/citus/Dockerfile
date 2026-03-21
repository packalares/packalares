FROM postgres:17.5
ARG VERSION=13.0.3
LABEL maintainer="Citus Data https://citusdata.com" \
      org.label-schema.name="Citus" \
      org.label-schema.description="Scalable PostgreSQL for multi-tenant and real-time workloads" \
      org.label-schema.url="https://www.citusdata.com" \
      org.label-schema.vcs-url="https://github.com/citusdata/citus" \
      org.label-schema.vendor="Citus Data, Inc." \
      org.label-schema.version=${VERSION} \
      org.label-schema.schema-version="1.0"

ENV CITUS_VERSION ${VERSION}.citus-1

# Install base deps
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       curl \
       lsb-release

# Install pig CLI (Pigsty package manager)
RUN curl -fsSL https://repo.pigsty.io/pig | bash

# Add pigsty and pgdg repos
RUN pig repo add pigsty pgdg -u

# Install PostgreSQL extensions via pig
RUN pig ext install citus -v 17 -y \
    && pig ext install hll -v 17 -y \
    && pig ext install topn -v 17 -y \
    && pig ext install pgvector -v 17 -y

# Install pgvecto.rs from GitHub (not in pigsty)
RUN arch=$(arch | sed s/aarch64/arm64/ | sed s/x86_64/amd64/) && \
    curl -sfLO https://github.com/tensorchord/pgvecto.rs/releases/download/v0.4.0/vectors-pg17_0.4.0_${arch}.deb && \
    apt-get install -y ./vectors-pg17_0.4.0_${arch}.deb && \
    rm vectors-pg17_0.4.0_${arch}.deb

# Install scws (Chinese word segmentation library, needed by zhparser)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential \
       postgresql-server-dev-17 \
       lbzip2 \
    && curl -sfLO http://www.xunsearch.com/scws/down/scws-1.2.3.tar.bz2 \
    && tar xf scws-1.2.3.tar.bz2 \
    && cd scws-1.2.3 \
    && ./configure \
    && make install \
    && echo "/usr/local/lib" | tee /etc/ld.so.conf.d/scws.conf \
    && ldconfig \
    && apt-get install -y equivs \
    && equivs-control scws \
    && sed -i 's/^# Version:.*/Version: 1.2.3/' scws \
    && sed -i 's/^Package:.*/Package: scws/' scws \
    && equivs-build scws \
    && dpkg -i scws_1.2.3_all.deb \
    && apt-get install -y locales-all \
    && locale-gen zh_CN.UTF-8 \
    && apt-get purge -y --auto-remove build-essential postgresql-server-dev-17 libpq-dev \
    && rm -rf scws-1.2.3.tar.bz2 scws-1.2.3

# Install zhparser via pig
RUN pig ext install zhparser -v 17 -y

# Install PostGIS from PGDG
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       postgresql-17-postgis-3 postgresql-17-postgis-3-scripts

# Install VectorChord from GitHub
RUN curl -sfLO https://github.com/tensorchord/VectorChord/releases/download/0.3.0/postgresql-17-vchord_0.3.0-1_$(dpkg --print-architecture).deb && \
    apt install -y ./postgresql-17-vchord_0.3.0-1_$(dpkg --print-architecture).deb && \
    rm postgresql-17-vchord_0.3.0-1_$(dpkg --print-architecture).deb

# Clean up
RUN apt-get clean && rm -rf /var/lib/apt/lists/*

# Add citus to default PostgreSQL config
RUN echo "shared_preload_libraries='citus,vectors.so,vchord.so'" >> /usr/share/postgresql/postgresql.conf.sample

# Add scripts to run after initdb
COPY docker/citus/001-create-citus-extension.sql /docker-entrypoint-initdb.d/

# Add health check script
COPY docker/citus/pg_healthcheck docker/citus/wait-for-manager.sh /
RUN chmod +x /wait-for-manager.sh

# entry point unsets PGPASSWORD, but we need it to connect to workers
RUN sed "/unset PGPASSWORD/d" -i /usr/local/bin/docker-entrypoint.sh

HEALTHCHECK --interval=4s --start-period=6s CMD ./pg_healthcheck
