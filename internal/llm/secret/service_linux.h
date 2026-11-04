#ifndef STEWARD_SECRET_SERVICE_H
#define STEWARD_SECRET_SERVICE_H

int steward_secret_init(void);
void *steward_secret_cancel_new(void);
void steward_secret_cancel(void *cancel);
void steward_secret_cancel_free(void *cancel);
char *steward_secret_get(const char *ref, void *cancel);
void steward_secret_free(char *value);
int steward_secret_put(const char *ref, const char *value, void *cancel);
int steward_secret_remove(const char *ref, void *cancel);

#endif
