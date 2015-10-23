FROM fedora:22

RUN dnf -y install postgresql-server postgresql hostname

RUN dnf clean all

RUN useradd -ms /bin/bash stolon

EXPOSE 5431 5432 6431

ENTRYPOINT ["/usr/local/bin/run.sh"]

ADD run.sh bin/stolon-keeper bin/stolon-sentinel bin/stolon-proxy /usr/local/bin/

RUN chmod +x /usr/local/bin/stolon-keeper /usr/local/bin/stolon-sentinel /usr/local/bin/stolon-proxy /usr/local/bin/run.sh

