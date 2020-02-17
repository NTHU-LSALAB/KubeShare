#include <nvml.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>

#define TO_STRING(str) #str
#define CALL_NVML_SUCCESS_FUNC(func, args...) \
    if (func(args) != NVML_SUCCESS) {\
        fprintf(stderr, TO_STRING(func unsuccessful.\n));\
        return 1;\
    }

int main(int argc, char *argv[]) {

    CALL_NVML_SUCCESS_FUNC(nvmlInit)

    unsigned int device_count;
    CALL_NVML_SUCCESS_FUNC(nvmlDeviceGetCount, &device_count)

    nvmlDevice_t deviceHandle;
    char *UUID = (char*)malloc(10240*sizeof(char));

    for (unsigned int i=0; i<device_count; ++i) {
        CALL_NVML_SUCCESS_FUNC(nvmlDeviceGetHandleByIndex, i, &deviceHandle)
        CALL_NVML_SUCCESS_FUNC(nvmlDeviceGetUUID, deviceHandle, UUID, 10240);
        printf("%s\n", UUID);
        fflush(stdout);
    }

    CALL_NVML_SUCCESS_FUNC(nvmlShutdown)

    pause();
}
