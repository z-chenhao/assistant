//go:build linux && cgo

#include "service_linux.h"
#include <dlfcn.h>
#include <stddef.h>
#include <string.h>

// libsecret owns Secret Service negotiation, encryption and desktop prompts.
// Load the runtime library so a missing optional credential dependency does not
// prevent unauthenticated local models or diagnostics from starting.
static struct {
 void *handle, *schema;
 void *(*schema_new)(const char *, int, ...);
 char *(*lookup)(const void *, void *, void **, ...);
 int (*store)(const void *, const char *, const char *, const char *, void *, void **, ...);
 int (*clear)(const void *, void *, void **, ...);
 void *(*search)(const void *, int, void *, void **, ...);
 void (*list_free_full)(void *, void (*)(void *));
 void (*password_free)(char *);
 void *(*cancel_new)(void);
 void (*cancel)(void *);
 void (*unref)(void *);
 void (*error_free)(void *);
} api;

int steward_secret_init(void) {
 api.handle = dlopen("libsecret-1.so.0", RTLD_NOW | RTLD_LOCAL);
 if (!api.handle) return 0;
#define LOAD(field, symbol) do { *(void **)(&api.field) = dlsym(api.handle, symbol); if (!api.field) goto fail; } while (0)
 LOAD(schema_new, "secret_schema_new");
 LOAD(lookup, "secret_password_lookup_sync");
 LOAD(store, "secret_password_store_sync");
 LOAD(clear, "secret_password_clear_sync");
 LOAD(search, "secret_password_search_sync");
 LOAD(list_free_full, "g_list_free_full");
 LOAD(password_free, "secret_password_free");
 LOAD(cancel_new, "g_cancellable_new");
 LOAD(cancel, "g_cancellable_cancel");
 LOAD(unref, "g_object_unref");
 LOAD(error_free, "g_error_free");
#undef LOAD
 // SECRET_SCHEMA_NONE and SECRET_SCHEMA_ATTRIBUTE_STRING are both zero.
 api.schema = api.schema_new("ai.steward.Credential", 0,
                            "service", 0, "credential", 0, NULL);
 if (api.schema) return 1;
fail:
 dlclose(api.handle);
 memset(&api, 0, sizeof(api));
 return 0;
}

void *steward_secret_cancel_new(void) { return api.cancel_new(); }
void steward_secret_cancel(void *cancel) { api.cancel(cancel); }
void steward_secret_cancel_free(void *cancel) { api.unref(cancel); }
void steward_secret_free(char *value) { api.password_free(value); }

char *steward_secret_get(const char *ref, void *cancel) {
 void *error = NULL;
 char *value = api.lookup(api.schema, cancel, &error,
                          "service", "steward", "credential", ref, NULL);
 if (error) {
  api.error_free(error);
  if (value) api.password_free(value);
  return NULL;
 }
 return value;
}

int steward_secret_put(const char *ref, const char *value, void *cancel) {
 void *error = NULL;
 int ok = api.store(api.schema, "default", "Steward credential", value,
                    cancel, &error, "service", "steward", "credential", ref, NULL);
 if (error) { api.error_free(error); return 0; }
 return ok;
}

int steward_secret_remove(const char *ref, void *cancel) {
 void *error = NULL;
 // libsecret can report no error when an unlock prompt is dismissed. Verify
 // absence with metadata-only SECRET_SEARCH_ALL (2), including locked items.
 // A missing item is idempotent; a surviving locked credential is not deleted.
 api.clear(api.schema, cancel, &error,
           "service", "steward", "credential", ref, NULL);
 if (error) { api.error_free(error); return 0; }
 void *remaining = api.search(api.schema, 2, cancel, &error,
                              "service", "steward", "credential", ref, NULL);
 int ok = !error && !remaining;
 if (remaining) api.list_free_full(remaining, api.unref);
 if (error) api.error_free(error);
 return ok;
}
