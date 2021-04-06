/**
 * Copyright 2020 Hung-Hsin Chen, LSA Lab, National Tsing Hua University
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifndef _CUHOOK_H_
#define _CUHOOK_H_
#include <cuda.h>

typedef enum HookSymbolsEnum {
  CU_HOOK_MEM_ALLOC,
  CU_HOOK_MEM_ALLOC_MANAGED,
  CU_HOOK_MEM_ALLOC_PITCH,
  CU_HOOK_MEM_FREE,
  CU_HOOK_ARRAY_CREATE,
  CU_HOOK_ARRAY3D_CREATE,
  CU_HOOK_MIPMAPPED_ARRAY_CREATE,
  CU_HOOK_ARRAY_DESTROY,
  CU_HOOK_MIPMAPPED_ARRAY_DESTROY,
  CU_HOOK_CTX_GET_CURRENT,
  CU_HOOK_CTX_SET_CURRENT,
  CU_HOOK_CTX_DESTROY,
  CU_HOOK_LAUNCH_KERNEL,
  CU_HOOK_LAUNCH_COOPERATIVE_KERNEL,
  CU_HOOK_DEVICE_TOTOAL_MEM,
  CU_HOOK_MEM_INFO,
  CU_HOOK_CTX_SYNC,
  CU_HOOK_MEMCPY_ATOH,
  CU_HOOK_MEMCPY_DTOH,
  CU_HOOK_MEMCPY_HTOA,
  CU_HOOK_MEMCPY_HTOD,
  NUM_HOOK_SYMBOLS,
} HookSymbols;

#endif /* _CUHOOK_H_ */
