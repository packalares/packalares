<template>
  <div class="iframe-root" @keydown="onKeydown" tabindex="0" ref="pageRef">
    <div class="iframe-sidebar">
      <div class="sidebar-brand">
        <div class="brand-icon" style="background:var(--accent-bold)">
          <q-icon name="sym_r_folder" size="18px" color="white" />
        </div>
        <div class="brand-info">
          <div class="brand-title">Files</div>
          <div class="brand-sub">File Manager</div>
        </div>
      </div>
      <div class="sidebar-divider"></div>
      <div class="sidebar-nav">
        <div
          v-for="f in sidebarFolders"
          :key="f.path"
          class="nav-item"
          :class="{ active: currentPath === f.path }"
          @click="navigateTo(f.path)"
        >
          <q-icon :name="'sym_r_' + f.icon" size="17px" class="nav-icon" />
          <span class="nav-text">{{ f.label }}</span>
        </div>
      </div>
    </div>
    <div class="files-main">
      <div class="files-toolbar">
        <q-btn flat dense round icon="sym_r_arrow_back" size="sm" :disable="historyIdx<=0" @click="goBack" />
        <q-btn flat dense round icon="sym_r_arrow_forward" size="sm" :disable="historyIdx>=history.length-1" @click="goForward" />
        <q-btn flat dense round icon="sym_r_arrow_upward" size="sm" :disable="currentPath==='/'" @click="goUp" />
        <div class="breadcrumb">
          <span class="crumb" @click="navigateTo('/')" title="Root">/</span>
          <template v-for="(c,i) in breadcrumbs" :key="i">
            <q-icon name="sym_r_chevron_right" size="14px" class="bread-sep" />
            <span class="crumb" :class="{'crumb-current': i === breadcrumbs.length - 1}" @click="navigateTo(c.path)" :title="c.path">{{c.name}}</span>
          </template>
        </div>
        <q-space />
        <!-- Sort -->
        <q-btn-dropdown flat dense icon="sym_r_sort" size="sm" no-caps class="toolbar-btn" dropdown-icon="none">
          <q-list dense style="min-width:160px">
            <q-item v-for="s in sortOptions" :key="s.key" clickable @click="setSortBy(s.key)" :class="{'text-primary': sortBy === s.key}">
              <q-item-section avatar style="min-width:28px">
                <q-icon :name="sortBy === s.key ? (sortAsc ? 'sym_r_arrow_upward' : 'sym_r_arrow_downward') : ''" size="16px" />
              </q-item-section>
              <q-item-section>
                <span style="font-size:13px">{{ s.label }}</span>
              </q-item-section>
            </q-item>
          </q-list>
        </q-btn-dropdown>
        <q-btn flat dense round :icon="viewMode==='grid'?'sym_r_view_list':'sym_r_grid_view'" size="sm" @click="viewMode=viewMode==='grid'?'list':'grid'" />
        <q-separator vertical class="q-mx-xs" style="height:20px" />
        <q-btn flat dense round icon="sym_r_create_new_folder" size="sm" @click="showNewFolderDialog" title="New Folder" />
        <q-btn flat dense round icon="sym_r_upload_file" size="sm" @click="triggerUpload" title="Upload File" />
        <q-btn flat dense round icon="sym_r_drive_folder_upload" size="sm" @click="triggerFolderUpload" title="Upload Folder" />
        <q-btn flat dense round icon="sym_r_delete" size="sm" color="negative" :disable="selectedIndices.size===0" @click="deleteSelected" title="Delete Selected" />
      </div>
      <!-- Drop zone overlay -->
      <div
        class="files-content"
        ref="contentRef"
        @click="onContentClick"
        @contextmenu.prevent.stop="onEmptyContext($event)"
        @dragenter.prevent="onDragEnter"
        @dragover.prevent="onDragOver"
        @dragleave.prevent="onDragLeave"
        @drop.prevent="onDrop"
        :class="{'drop-active': dropActive}"
      >
        <div v-if="dropActive" class="drop-overlay">
          <q-icon name="sym_r_upload" size="48px" />
          <div>Drop files here to upload</div>
        </div>
        <!-- Grid View -->
        <div v-if="viewMode==='grid'" class="file-grid">
          <div
            v-for="(file,idx) in sortedFiles"
            :key="file.name"
            class="file-card"
            :class="{selected: selectedIndices.has(idx), 'cut-item': isCutItem(file)}"
            @click.stop="onItemClick($event, idx)"
            @dblclick="openFile(file)"
            @contextmenu.prevent.stop="onContext($event, file, idx)"
          >
            <!-- Thumbnail for images -->
            <div v-if="isImage(file.name) && !file.isDir" class="file-thumb">
              <img :src="getThumbUrl(file)" :alt="file.name" loading="lazy" @error="($event.target as HTMLImageElement).style.display='none'" />
            </div>
            <div v-else class="file-icon-wrap" :class="file.isDir ? 'icon-folder' : iconClass(file.name)">
              <q-icon :name="file.isDir?'sym_r_folder':fileIcon(file.name)" size="28px" />
            </div>
            <!-- Inline rename -->
            <input
              v-if="renamingIdx === idx"
              class="rename-input"
              ref="renameInputRef"
              v-model="renameValue"
              @blur="commitRename"
              @keydown.enter.prevent="commitRename"
              @keydown.escape.prevent="cancelRename"
              @click.stop
              @dblclick.stop
            />
            <div v-else class="file-name" :title="file.name">{{file.name}}</div>
            <div v-if="!file.isDir" class="file-meta">{{fmtSize(file.size)}}</div>
            <div class="file-date">{{fmtDate(file.modified)}}</div>
          </div>
        </div>
        <!-- List View -->
        <q-list v-else dense separator class="list-view">
          <q-item class="list-header">
            <q-item-section avatar style="min-width:36px"></q-item-section>
            <q-item-section><span class="list-header-text">Name</span></q-item-section>
            <q-item-section side style="min-width:90px;text-align:right"><span class="list-header-text">Size</span></q-item-section>
            <q-item-section side style="min-width:140px"><span class="list-header-text">Modified</span></q-item-section>
            <q-item-section side style="min-width:80px"><span class="list-header-text">Type</span></q-item-section>
          </q-item>
          <q-item
            v-for="(file,idx) in sortedFiles"
            :key="file.name"
            clickable
            :class="{'list-item-selected': selectedIndices.has(idx), 'cut-item': isCutItem(file)}"
            @click.stop="onItemClick($event, idx)"
            @dblclick="openFile(file)"
            @contextmenu.prevent.stop="onContext($event, file, idx)"
            class="list-item"
          >
            <q-item-section avatar style="min-width:36px">
              <div v-if="isImage(file.name) && !file.isDir" class="list-thumb">
                <img :src="getThumbUrl(file)" :alt="file.name" loading="lazy" />
              </div>
              <q-icon v-else :name="file.isDir?'sym_r_folder':fileIcon(file.name)" :color="file.isDir?'amber':'grey'" size="20px" />
            </q-item-section>
            <q-item-section>
              <input
                v-if="renamingIdx === idx"
                class="rename-input rename-input-list"
                ref="renameInputRef"
                v-model="renameValue"
                @blur="commitRename"
                @keydown.enter.prevent="commitRename"
                @keydown.escape.prevent="cancelRename"
                @click.stop
                @dblclick.stop
              />
              <span v-else class="list-name">{{file.name}}</span>
            </q-item-section>
            <q-item-section side style="min-width:90px;text-align:right"><span class="list-meta">{{file.isDir ? (file.numFiles||0)+' items' : fmtSize(file.size)}}</span></q-item-section>
            <q-item-section side style="min-width:140px"><span class="list-meta">{{fmtDate(file.modified)}}</span></q-item-section>
            <q-item-section side style="min-width:80px"><span class="list-meta">{{file.isDir ? 'Folder' : getExt(file.name).toUpperCase() || 'File'}}</span></q-item-section>
          </q-item>
        </q-list>
        <div v-if="!sortedFiles.length&&!loading" class="empty-state">
          <q-icon name="sym_r_folder_open" size="48px" color="grey-7" />
          <div class="q-mt-sm">Empty folder</div>
          <div class="q-mt-xs" style="font-size:11px;color:var(--ink-3)">Drop files here or use the upload button</div>
        </div>
      </div>
      <div class="files-status">
        {{sortedFiles.length}} items
        <span v-if="selectedIndices.size === 1"> &mdash; {{getFirstSelected()?.name}}</span>
        <span v-else-if="selectedIndices.size > 1"> &mdash; {{selectedIndices.size}} selected</span>
        <span v-if="selectedIndices.size === 1 && !getFirstSelected()?.isDir"> ({{fmtSize(getFirstSelected()?.size)}})</span>
        <span v-if="clipboard.items.length" class="status-clipboard">
          &nbsp;| {{clipboard.items.length}} {{clipboard.mode === 'cut' ? 'cut' : 'copied'}}
        </span>
        <q-space />
        <span class="status-path">{{currentPath}}</span>
      </div>
    </div>
    <!-- New Folder Dialog -->
    <q-dialog v-model="showNewFolder">
      <q-card style="min-width:320px" dark>
        <q-card-section class="text-subtitle1" style="font-weight:600">New Folder</q-card-section>
        <q-card-section>
          <q-input v-model="newFolderName" label="Folder name" dense dark outlined autofocus @keydown.enter="createFolder" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup class="btn-ghost" />
          <q-btn unelevated label="Create" class="btn-primary" @click="createFolder" />
        </q-card-actions>
      </q-card>
    </q-dialog>
    <!-- Context Menu -->
    <teleport to="body">
    <div v-if="contextMenu.show" class="context-menu-overlay" @click="contextMenu.show=false" @contextmenu.prevent="contextMenu.show=false">
    <div class="context-menu" :style="contextMenu.style" @click="contextMenu.show=false">
      <q-list dense style="min-width:180px">
        <template v-if="contextMenu.type === 'file'">
          <q-item clickable @click="openFile(contextMenu.file!)">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_open_in_new" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Open</span></q-item-section>
          </q-item>
          <q-item v-if="!contextMenu.file?.isDir" clickable @click="downloadFile(contextMenu.file!)">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_download" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Download</span></q-item-section>
          </q-item>
          <q-separator />
          <q-item clickable @click="startRename(contextMenu.fileIdx!)">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_edit" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Rename</span></q-item-section>
            <q-item-section side><span class="ctx-shortcut">F2</span></q-item-section>
          </q-item>
          <q-item clickable @click="clipboardCopy">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_content_copy" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Copy</span></q-item-section>
            <q-item-section side><span class="ctx-shortcut">Ctrl+C</span></q-item-section>
          </q-item>
          <q-item clickable @click="clipboardCut">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_content_cut" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Cut</span></q-item-section>
            <q-item-section side><span class="ctx-shortcut">Ctrl+X</span></q-item-section>
          </q-item>
          <q-separator />
          <q-item clickable @click="deleteSelectedItems">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_delete" size="16px" color="negative" /></q-item-section>
            <q-item-section><span class="ctx-label text-negative">Delete</span></q-item-section>
            <q-item-section side><span class="ctx-shortcut">Del</span></q-item-section>
          </q-item>
        </template>
        <template v-else>
          <!-- Empty space context menu -->
          <q-item clickable @click="showNewFolderDialog">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_create_new_folder" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">New Folder</span></q-item-section>
          </q-item>
          <q-item clickable @click="triggerUpload">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_upload_file" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Upload File</span></q-item-section>
          </q-item>
          <q-item clickable @click="triggerFolderUpload">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_drive_folder_upload" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Upload Folder</span></q-item-section>
          </q-item>
          <q-separator v-if="clipboard.items.length" />
          <q-item v-if="clipboard.items.length" clickable @click="clipboardPaste">
            <q-item-section avatar style="min-width:28px"><q-icon name="sym_r_content_paste" size="16px" /></q-item-section>
            <q-item-section><span class="ctx-label">Paste ({{clipboard.items.length}} items)</span></q-item-section>
            <q-item-section side><span class="ctx-shortcut">Ctrl+V</span></q-item-section>
          </q-item>
        </template>
      </q-list>
    </div>
    </div>
    </teleport>
    <input ref="uploadRef" type="file" multiple style="display:none" @change="handleUpload" />
    <input ref="folderUploadRef" type="file" multiple style="display:none" webkitdirectory @change="handleFolderUpload" />
  </div>
</template>
<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick, reactive, watch } from 'vue';
import { api, getApiBase } from 'boot/axios';
import { useQuasar } from 'quasar';

const $q = useQuasar();
const pageRef = ref<HTMLElement|null>(null);
const contentRef = ref<HTMLElement|null>(null);

// ----- State -----
const currentPath = ref('/');
const files = ref<any[]>([]);
const loading = ref(false);
const viewMode = ref<'grid'|'list'>('list');
const selectedIndices = ref<Set<number>>(new Set());
const lastClickedIdx = ref(-1);
const history = ref(['/']);
const historyIdx = ref(0);
const showNewFolder = ref(false);
const newFolderName = ref('');
const uploadRef = ref<HTMLInputElement|null>(null);
const folderUploadRef = ref<HTMLInputElement|null>(null);

// Sort state
const sortBy = ref<'name'|'size'|'date'|'type'>('name');
const sortAsc = ref(true);
const sortOptions = [
  { key: 'name' as const, label: 'Name' },
  { key: 'size' as const, label: 'Size' },
  { key: 'date' as const, label: 'Date Modified' },
  { key: 'type' as const, label: 'Type' },
];

// Clipboard state
const clipboard = reactive<{ items: { name: string; path: string; isDir: boolean }[]; mode: 'copy'|'cut'; sourcePath: string }>({
  items: [],
  mode: 'copy',
  sourcePath: '/',
});

// Rename state
const renamingIdx = ref(-1);
const renameValue = ref('');
const renameOriginal = ref('');
const renameInputRef = ref<HTMLInputElement[]|null>(null);

// Drag & drop state
const dropActive = ref(false);
let dragCounter = 0;

// Context menu state
const contextMenu = reactive<{
  show: boolean;
  style: string;
  type: 'file'|'empty';
  file: any;
  fileIdx: number|null;
}>({
  show: false,
  style: '',
  type: 'empty',
  file: null,
  fileIdx: null,
});

const sidebarFolders = ref<{path:string;label:string;icon:string}[]>([
  {path:'/',label:'All Files',icon:'folder'},
]);

const folderIcons: Record<string, string> = {
  Home: 'home', Documents: 'description', Downloads: 'download',
  Pictures: 'image', Videos: 'videocam', Music: 'music_note',
  Mounts: 'lan', Desktop: 'desktop_windows',
};

async function loadSidebar() {
  try {
    const root: any = await api.get('/api/files/resources/');
    const rootItems = (root.items || []).filter((f: any) => f.isDir);
    const entries: {path:string;label:string;icon:string}[] = [
      {path:'/',label:'All Files',icon:'folder'},
    ];
    for (const f of rootItems) {
      entries.push({path:'/'+f.name, label:f.name, icon: folderIcons[f.name] || 'folder'});
      // Load subfolders for Home
      if (f.name === 'Home') {
        try {
          const home: any = await api.get('/api/files/resources/Home');
          const homeItems = (home.items || []).filter((h: any) => h.isDir);
          for (const h of homeItems) {
            entries.push({path:'/Home/'+h.name, label:h.name, icon: folderIcons[h.name] || 'folder'});
          }
        } catch {}
      }
    }
    sidebarFolders.value = entries;
  } catch {}
}

// ----- Computed -----
const breadcrumbs = computed(() => {
  const p = currentPath.value.split('/').filter(Boolean);
  return p.map((n, i) => ({ name: n, path: '/' + p.slice(0, i + 1).join('/') }));
});

const sortedFiles = computed(() => {
  const arr = [...files.value];
  arr.sort((a, b) => {
    // Folders always first
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    let cmp = 0;
    switch (sortBy.value) {
      case 'name':
        cmp = a.name.localeCompare(b.name);
        break;
      case 'size':
        cmp = (a.size || 0) - (b.size || 0);
        break;
      case 'date':
        cmp = new Date(a.modified || 0).getTime() - new Date(b.modified || 0).getTime();
        break;
      case 'type': {
        const ea = getExt(a.name);
        const eb = getExt(b.name);
        cmp = ea.localeCompare(eb);
        break;
      }
    }
    return sortAsc.value ? cmp : -cmp;
  });
  return arr;
});

// ----- Core file operations -----
async function loadFiles(path: string) {
  loading.value = true;
  selectedIndices.value = new Set();
  lastClickedIdx.value = -1;
  renamingIdx.value = -1;
  try {
    const d: any = await api.get('/api/files/resources' + path);
    files.value = d.items || [];
  } catch {
    files.value = [];
  }
  loading.value = false;
}

function navigateTo(p: string) {
  currentPath.value = p;
  history.value = history.value.slice(0, historyIdx.value + 1);
  history.value.push(p);
  historyIdx.value = history.value.length - 1;
  loadFiles(p);
}

function goBack() {
  if (historyIdx.value > 0) {
    historyIdx.value--;
    currentPath.value = history.value[historyIdx.value];
    loadFiles(currentPath.value);
  }
}

function goForward() {
  if (historyIdx.value < history.value.length - 1) {
    historyIdx.value++;
    currentPath.value = history.value[historyIdx.value];
    loadFiles(currentPath.value);
  }
}

function goUp() {
  if (currentPath.value === '/') return;
  const p = currentPath.value.split('/').filter(Boolean);
  p.pop();
  navigateTo(p.length ? '/' + p.join('/') : '/');
}

function buildPath(file: any): string {
  return (currentPath.value === '/' ? '' : currentPath.value) + '/' + file.name;
}

function openFile(f: any) {
  if (f.isDir) {
    navigateTo(currentPath.value === '/' ? '/' + f.name : currentPath.value + '/' + f.name);
  } else {
    window.open(getApiBase() + '/api/files/raw' + buildPath(f), '_blank');
  }
}

function downloadFile(f: any) {
  const a = document.createElement('a');
  a.href = getApiBase() + '/api/files/raw' + buildPath(f);
  a.download = f.name;
  a.click();
}

// ----- Selection -----
function onItemClick(e: MouseEvent, idx: number) {
  if (renamingIdx.value >= 0 && renamingIdx.value !== idx) {
    commitRename();
  }

  if (e.ctrlKey || e.metaKey) {
    // Toggle individual item
    const s = new Set(selectedIndices.value);
    if (s.has(idx)) s.delete(idx); else s.add(idx);
    selectedIndices.value = s;
    lastClickedIdx.value = idx;
  } else if (e.shiftKey && lastClickedIdx.value >= 0) {
    // Range select
    const s = new Set(selectedIndices.value);
    const start = Math.min(lastClickedIdx.value, idx);
    const end = Math.max(lastClickedIdx.value, idx);
    for (let i = start; i <= end; i++) s.add(i);
    selectedIndices.value = s;
  } else {
    // Single select
    selectedIndices.value = new Set([idx]);
    lastClickedIdx.value = idx;
  }
}

function onContentClick() {
  selectedIndices.value = new Set();
  lastClickedIdx.value = -1;
  if (renamingIdx.value >= 0) cancelRename();
}

function selectAll() {
  const s = new Set<number>();
  for (let i = 0; i < sortedFiles.value.length; i++) s.add(i);
  selectedIndices.value = s;
}

function getFirstSelected(): any {
  if (selectedIndices.value.size === 0) return null;
  const idx = [...selectedIndices.value][0];
  return sortedFiles.value[idx] || null;
}

function getSelectedFiles(): any[] {
  return [...selectedIndices.value].sort().map(i => sortedFiles.value[i]).filter(Boolean);
}

// ----- Context menu -----
function onContext(e: MouseEvent, f: any, idx: number) {
  // If the right-clicked item is not already selected, select only it
  if (!selectedIndices.value.has(idx)) {
    selectedIndices.value = new Set([idx]);
    lastClickedIdx.value = idx;
  }
  contextMenu.type = 'file';
  contextMenu.file = f;
  contextMenu.fileIdx = idx;
  showContextAt(e);
}

function onEmptyContext(e: MouseEvent) {
  selectedIndices.value = new Set();
  contextMenu.type = 'empty';
  contextMenu.file = null;
  contextMenu.fileIdx = null;
  showContextAt(e);
}

function showContextAt(e: MouseEvent) {
  contextMenu.style = `position:fixed;left:${e.clientX}px;top:${e.clientY}px;z-index:9999`;
  contextMenu.show = true;
}

function ctxAction(fn: () => void) {
  contextMenu.show = false;
  fn();
}

// ----- Sort -----
function setSortBy(key: 'name'|'size'|'date'|'type') {
  if (sortBy.value === key) {
    sortAsc.value = !sortAsc.value;
  } else {
    sortBy.value = key;
    sortAsc.value = true;
  }
}

// ----- Create / Upload / Delete -----
function showNewFolderDialog() {
  newFolderName.value = '';
  showNewFolder.value = true;
}

async function createFolder() {
  if (!newFolderName.value.trim()) return;
  try {
    await api.put('/api/files/resources' + currentPath.value + '/' + newFolderName.value.trim() + '/', {});
    showNewFolder.value = false;
    newFolderName.value = '';
    $q.notify({ type: 'positive', message: 'Folder created' });
    loadFiles(currentPath.value);
  } catch {
    $q.notify({ type: 'negative', message: 'Failed to create folder' });
  }
}

function triggerUpload() {
  uploadRef.value?.click();
}

function triggerFolderUpload() {
  folderUploadRef.value?.click();
}

async function handleUpload(e: Event) {
  const input = e.target as HTMLInputElement;
  if (!input.files?.length) return;
  await uploadFileList(Array.from(input.files));
  input.value = '';
}

async function handleFolderUpload(e: Event) {
  const input = e.target as HTMLInputElement;
  if (!input.files?.length) return;
  await uploadFileList(Array.from(input.files), true);
  input.value = '';
}

async function uploadFileList(fileList: File[], preservePaths = false) {
  let ok = 0;
  let fail = 0;
  for (const f of fileList) {
    const fd = new FormData();
    fd.append('file', f);
    // For folder uploads, webkitRelativePath has the relative path
    let uploadPath = currentPath.value;
    if (preservePaths && (f as any).webkitRelativePath) {
      const relParts = (f as any).webkitRelativePath.split('/');
      if (relParts.length > 1) {
        // Create subdirectories as needed; the file goes into the last segment
        const dirParts = relParts.slice(0, -1);
        const subDir = dirParts.join('/');
        uploadPath = currentPath.value + '/' + subDir;
        // Ensure the directory exists
        try {
          await api.put('/api/files/resources' + uploadPath + '/', {});
        } catch { /* may already exist */ }
      }
    }
    const fileName = f.name;
    try {
      await api.post('/api/files/resources' + uploadPath + '/' + fileName, fd);
      ok++;
    } catch {
      fail++;
    }
  }
  if (ok > 0) $q.notify({ type: 'positive', message: `Uploaded ${ok} file${ok > 1 ? 's' : ''}` });
  if (fail > 0) $q.notify({ type: 'negative', message: `Failed to upload ${fail} file${fail > 1 ? 's' : ''}` });
  loadFiles(currentPath.value);
}

async function deleteFile(f: any) {
  try {
    await api.delete('/api/files/resources' + buildPath(f));
    return true;
  } catch {
    return false;
  }
}

function deleteSelected() {
  deleteSelectedItems();
}

function deleteSelectedItems() {
  const items = getSelectedFiles();
  if (!items.length) return;
  const msg = items.length === 1
    ? `Delete "${items[0].name}"?`
    : `Delete ${items.length} items?`;
  $q.dialog({
    title: 'Delete',
    message: msg,
    cancel: true,
    persistent: true,
    dark: true,
    ok: { label: 'Delete', flat: true, color: 'negative' },
  }).onOk(async () => {
    let fail = 0;
    for (const f of items) {
      const ok = await deleteFile(f);
      if (!ok) fail++;
    }
    if (fail > 0) $q.notify({ type: 'negative', message: `Failed to delete ${fail} item${fail > 1 ? 's' : ''}` });
    else $q.notify({ type: 'positive', message: items.length === 1 ? 'Deleted' : `Deleted ${items.length} items` });
    loadFiles(currentPath.value);
  });
}

// ----- Rename -----
function startRename(idx: number) {
  const file = sortedFiles.value[idx];
  if (!file) return;
  renamingIdx.value = idx;
  renameOriginal.value = file.name;
  renameValue.value = file.name;
  nextTick(() => {
    const inputs = renameInputRef.value;
    if (inputs && inputs.length > 0) {
      const inp = inputs[0];
      inp.focus();
      // Select filename without extension for files
      if (!file.isDir) {
        const dot = file.name.lastIndexOf('.');
        if (dot > 0) {
          inp.setSelectionRange(0, dot);
        } else {
          inp.select();
        }
      } else {
        inp.select();
      }
    }
  });
}

function cancelRename() {
  renamingIdx.value = -1;
  renameValue.value = '';
  renameOriginal.value = '';
}

async function commitRename() {
  if (renamingIdx.value < 0) return;
  const file = sortedFiles.value[renamingIdx.value];
  const newName = renameValue.value.trim();
  renamingIdx.value = -1;

  if (!newName || newName === renameOriginal.value || !file) {
    return;
  }

  // Use PUT to rename (move) the file
  try {
    const oldPath = buildPath(file);
    const newPath = (currentPath.value === '/' ? '' : currentPath.value) + '/' + newName;
    await api.put('/api/files/resources' + oldPath, { action: 'rename', destination: newPath });
    $q.notify({ type: 'positive', message: 'Renamed' });
    loadFiles(currentPath.value);
  } catch {
    $q.notify({ type: 'negative', message: 'Rename failed' });
  }
}

// ----- Clipboard (Copy/Cut/Paste) -----
function clipboardCopy() {
  const items = getSelectedFiles();
  if (!items.length) return;
  clipboard.items = items.map(f => ({ name: f.name, path: buildPath(f), isDir: f.isDir }));
  clipboard.mode = 'copy';
  clipboard.sourcePath = currentPath.value;
  $q.notify({ type: 'info', message: `Copied ${items.length} item${items.length > 1 ? 's' : ''}`, timeout: 1500 });
}

function clipboardCut() {
  const items = getSelectedFiles();
  if (!items.length) return;
  clipboard.items = items.map(f => ({ name: f.name, path: buildPath(f), isDir: f.isDir }));
  clipboard.mode = 'cut';
  clipboard.sourcePath = currentPath.value;
  $q.notify({ type: 'info', message: `Cut ${items.length} item${items.length > 1 ? 's' : ''}`, timeout: 1500 });
}

async function clipboardPaste() {
  if (!clipboard.items.length) return;
  let ok = 0;
  let fail = 0;
  for (const item of clipboard.items) {
    const dest = (currentPath.value === '/' ? '' : currentPath.value) + '/' + item.name;
    try {
      await api.put('/api/files/resources' + item.path, {
        action: clipboard.mode === 'cut' ? 'move' : 'copy',
        destination: dest,
      });
      ok++;
    } catch {
      fail++;
    }
  }
  if (ok > 0) $q.notify({ type: 'positive', message: `${clipboard.mode === 'cut' ? 'Moved' : 'Copied'} ${ok} item${ok > 1 ? 's' : ''}` });
  if (fail > 0) $q.notify({ type: 'negative', message: `Failed: ${fail} item${fail > 1 ? 's' : ''}` });
  if (clipboard.mode === 'cut') {
    clipboard.items = [];
  }
  loadFiles(currentPath.value);
}

function isCutItem(file: any): boolean {
  if (clipboard.mode !== 'cut') return false;
  if (clipboard.sourcePath !== currentPath.value) return false;
  return clipboard.items.some(c => c.name === file.name);
}

// ----- Drag and drop upload -----
function onDragEnter(e: DragEvent) {
  dragCounter++;
  if (e.dataTransfer?.types.includes('Files')) {
    dropActive.value = true;
  }
}

function onDragOver(e: DragEvent) {
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
}

function onDragLeave() {
  dragCounter--;
  if (dragCounter <= 0) {
    dragCounter = 0;
    dropActive.value = false;
  }
}

async function onDrop(e: DragEvent) {
  dragCounter = 0;
  dropActive.value = false;
  if (!e.dataTransfer) return;

  // Try to get folder entries via webkitGetAsEntry
  const items = e.dataTransfer.items;
  if (items && items.length > 0) {
    const allFiles: File[] = [];
    const entries: any[] = [];
    for (let i = 0; i < items.length; i++) {
      const entry = items[i].webkitGetAsEntry?.();
      if (entry) entries.push(entry);
    }
    if (entries.length > 0) {
      for (const entry of entries) {
        await readEntry(entry, '', allFiles);
      }
      if (allFiles.length > 0) {
        await uploadDroppedFiles(allFiles);
      }
      return;
    }
  }
  // Fallback: plain files
  if (e.dataTransfer.files.length > 0) {
    await uploadFileList(Array.from(e.dataTransfer.files));
  }
}

async function readEntry(entry: any, basePath: string, result: File[]) {
  if (entry.isFile) {
    const file: File = await new Promise((resolve, reject) => entry.file(resolve, reject));
    // Attach relative path for folder reconstruction
    Object.defineProperty(file, '_relativePath', { value: basePath + file.name, writable: false });
    result.push(file);
  } else if (entry.isDirectory) {
    const reader = entry.createReader();
    const entries: any[] = await new Promise((resolve, reject) => {
      const all: any[] = [];
      const readBatch = () => {
        reader.readEntries((batch: any[]) => {
          if (batch.length === 0) {
            resolve(all);
          } else {
            all.push(...batch);
            readBatch();
          }
        }, reject);
      };
      readBatch();
    });
    for (const child of entries) {
      await readEntry(child, basePath + entry.name + '/', result);
    }
  }
}

async function uploadDroppedFiles(fileList: File[]) {
  let ok = 0;
  let fail = 0;
  for (const f of fileList) {
    const fd = new FormData();
    fd.append('file', f);
    const relPath = (f as any)._relativePath || f.name;
    const parts = relPath.split('/');
    let uploadPath = currentPath.value;
    if (parts.length > 1) {
      const dirParts = parts.slice(0, -1);
      uploadPath = currentPath.value + '/' + dirParts.join('/');
      try {
        await api.put('/api/files/resources' + uploadPath + '/', {});
      } catch { /* may already exist */ }
    }
    try {
      await api.post('/api/files/resources' + uploadPath + '/' + parts[parts.length - 1], fd);
      ok++;
    } catch {
      fail++;
    }
  }
  if (ok > 0) $q.notify({ type: 'positive', message: `Uploaded ${ok} file${ok > 1 ? 's' : ''}` });
  if (fail > 0) $q.notify({ type: 'negative', message: `Failed to upload ${fail} file${fail > 1 ? 's' : ''}` });
  loadFiles(currentPath.value);
}

// ----- Keyboard shortcuts -----
function onKeydown(e: KeyboardEvent) {
  // Don't handle shortcuts if we're in a dialog or renaming
  if (showNewFolder.value) return;
  if (renamingIdx.value >= 0) return;

  const ctrl = e.ctrlKey || e.metaKey;

  // Ctrl+A: select all
  if (ctrl && e.key === 'a') {
    e.preventDefault();
    selectAll();
    return;
  }

  // Ctrl+C: copy
  if (ctrl && e.key === 'c') {
    e.preventDefault();
    clipboardCopy();
    return;
  }

  // Ctrl+X: cut
  if (ctrl && e.key === 'x') {
    e.preventDefault();
    clipboardCut();
    return;
  }

  // Ctrl+V: paste
  if (ctrl && e.key === 'v') {
    e.preventDefault();
    clipboardPaste();
    return;
  }

  // Delete key
  if (e.key === 'Delete') {
    e.preventDefault();
    deleteSelectedItems();
    return;
  }

  // F2: rename
  if (e.key === 'F2') {
    e.preventDefault();
    if (selectedIndices.value.size === 1) {
      startRename([...selectedIndices.value][0]);
    }
    return;
  }

  // Enter: open selected
  if (e.key === 'Enter') {
    e.preventDefault();
    if (selectedIndices.value.size === 1) {
      const file = sortedFiles.value[[...selectedIndices.value][0]];
      if (file) openFile(file);
    }
    return;
  }

  // Backspace: go up
  if (e.key === 'Backspace') {
    e.preventDefault();
    goUp();
    return;
  }
}

// ----- File type helpers -----
function getExt(name: string): string {
  return name.split('.').pop()?.toLowerCase() || '';
}

function isImage(name: string): boolean {
  const ext = getExt(name);
  return ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg', 'ico'].includes(ext);
}

function getThumbUrl(file: any): string {
  return getApiBase() + '/api/files/raw' + buildPath(file);
}

function fileIcon(n: string): string {
  const e = getExt(n);
  const m: Record<string, string> = {
    pdf: 'sym_r_picture_as_pdf',
    doc: 'sym_r_description', docx: 'sym_r_description', odt: 'sym_r_description', rtf: 'sym_r_description',
    xls: 'sym_r_table_chart', xlsx: 'sym_r_table_chart', ods: 'sym_r_table_chart', csv: 'sym_r_table_chart',
    ppt: 'sym_r_slideshow', pptx: 'sym_r_slideshow', odp: 'sym_r_slideshow',
    txt: 'sym_r_article', log: 'sym_r_article',
    png: 'sym_r_image', jpg: 'sym_r_image', jpeg: 'sym_r_image', gif: 'sym_r_image',
    webp: 'sym_r_image', svg: 'sym_r_image', bmp: 'sym_r_image', ico: 'sym_r_image',
    mp4: 'sym_r_movie', mkv: 'sym_r_movie', avi: 'sym_r_movie', mov: 'sym_r_movie', wmv: 'sym_r_movie', webm: 'sym_r_movie',
    mp3: 'sym_r_audio_file', flac: 'sym_r_audio_file', wav: 'sym_r_audio_file', ogg: 'sym_r_audio_file', aac: 'sym_r_audio_file', wma: 'sym_r_audio_file',
    zip: 'sym_r_folder_zip', tar: 'sym_r_folder_zip', gz: 'sym_r_folder_zip', '7z': 'sym_r_folder_zip', rar: 'sym_r_folder_zip', bz2: 'sym_r_folder_zip', xz: 'sym_r_folder_zip',
    iso: 'sym_r_album', dmg: 'sym_r_album', img: 'sym_r_album',
    js: 'sym_r_code', ts: 'sym_r_code', jsx: 'sym_r_code', tsx: 'sym_r_code',
    py: 'sym_r_code', go: 'sym_r_code', rs: 'sym_r_code', rb: 'sym_r_code', java: 'sym_r_code', c: 'sym_r_code', cpp: 'sym_r_code', h: 'sym_r_code',
    html: 'sym_r_code', css: 'sym_r_code', scss: 'sym_r_code', less: 'sym_r_code',
    json: 'sym_r_data_object', yaml: 'sym_r_data_object', yml: 'sym_r_data_object', toml: 'sym_r_data_object', xml: 'sym_r_data_object',
    md: 'sym_r_markdown', sh: 'sym_r_terminal', bash: 'sym_r_terminal', zsh: 'sym_r_terminal',
    sql: 'sym_r_database', db: 'sym_r_database', sqlite: 'sym_r_database',
    exe: 'sym_r_apps', msi: 'sym_r_apps', deb: 'sym_r_apps', rpm: 'sym_r_apps', appimage: 'sym_r_apps',
    ttf: 'sym_r_font_download', otf: 'sym_r_font_download', woff: 'sym_r_font_download', woff2: 'sym_r_font_download',
    conf: 'sym_r_settings', cfg: 'sym_r_settings', ini: 'sym_r_settings', env: 'sym_r_settings',
    key: 'sym_r_key', pem: 'sym_r_key', crt: 'sym_r_key', cer: 'sym_r_key',
  };
  return m[e] || 'sym_r_draft';
}

function iconClass(name: string): string {
  const ext = getExt(name);
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg', 'ico'].includes(ext)) return 'icon-image';
  if (['mp4', 'mkv', 'avi', 'mov', 'wmv', 'webm'].includes(ext)) return 'icon-video';
  if (['mp3', 'flac', 'wav', 'ogg', 'aac', 'wma'].includes(ext)) return 'icon-audio';
  if (['pdf'].includes(ext)) return 'icon-pdf';
  if (['zip', 'tar', 'gz', '7z', 'rar', 'bz2', 'xz'].includes(ext)) return 'icon-archive';
  if (['js', 'ts', 'jsx', 'tsx', 'py', 'go', 'rs', 'rb', 'java', 'c', 'cpp', 'h', 'html', 'css', 'scss', 'less', 'sh', 'bash', 'zsh'].includes(ext)) return 'icon-code';
  return 'icon-file';
}

function fmtSize(b: number): string {
  if (!b) return '';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (b >= 1024 && i < 4) { b /= 1024; i++; }
  return b.toFixed(i > 0 ? 1 : 0) + ' ' + u[i];
}

function fmtDate(d: string): string {
  return d ? new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '';
}

// ----- Lifecycle -----
onMounted(() => {
  loadSidebar();
  loadFiles(currentPath.value);
  nextTick(() => pageRef.value?.focus());
});
</script>
<style scoped lang="scss">
// iframe-root, iframe-sidebar, sidebar-brand, sidebar-nav, nav-item from components.scss

.files-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }

.files-toolbar {
  height: 40px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 10px;
  border-bottom: 1px solid var(--separator);
  flex-shrink: 0;
  background: rgba(255, 255, 255, 0.015);
}

.toolbar-btn {
  font-size: 13px;
}

.breadcrumb {
  display: flex;
  align-items: center;
  font-size: 13px;
  margin-left: 6px;
  overflow: hidden;
  white-space: nowrap;
}

.crumb {
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--ink-2);
  font-weight: 500;
  flex-shrink: 0;

  &:hover { background: var(--glass); color: var(--ink-1); }
}

.crumb-current {
  color: var(--ink-1);
  font-weight: 600;
}

.bread-sep {
  color: var(--ink-3);
  flex-shrink: 0;
  margin: 0 1px;
}

.files-content {
  flex: 1;
  overflow-y: auto;
  padding: 14px;
  position: relative;
}

.drop-active {
  &::after {
    content: '';
    position: absolute;
    inset: 8px;
    border: 2px dashed var(--accent);
    border-radius: 12px;
    background: rgba(59, 130, 246, 0.06);
    pointer-events: none;
    z-index: 10;
  }
}

.drop-overlay {
  position: absolute;
  inset: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--accent);
  font-size: 15px;
  font-weight: 600;
  z-index: 11;
  pointer-events: none;
}

.file-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: 4px;
}

.file-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 12px 6px 8px;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.1s ease;
  position: relative;

  &:hover { background: var(--glass); }
  &.selected { background: var(--accent-soft); }
  &.cut-item { opacity: 0.45; }
}

.file-icon-wrap {
  width: 42px;
  height: 42px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;

  &.icon-folder { background: rgba(251,191,36,0.10); color: #fbbf24; }
  &.icon-file { background: var(--hover-bg); color: var(--ink-3); }
  &.icon-image { background: rgba(59, 130, 246, 0.08); color: #3b82f6; }
  &.icon-video { background: rgba(168, 85, 247, 0.08); color: #a855f7; }
  &.icon-audio { background: rgba(236, 72, 153, 0.08); color: #ec4899; }
  &.icon-pdf { background: rgba(239, 68, 68, 0.08); color: #ef4444; }
  &.icon-archive { background: rgba(234, 179, 8, 0.08); color: #eab308; }
  &.icon-code { background: rgba(16, 185, 129, 0.08); color: #10b981; }
}

.file-thumb {
  width: 68px;
  height: 52px;
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--subtle-bg);

  img {
    max-width: 100%;
    max-height: 100%;
    object-fit: cover;
    border-radius: 6px;
  }
}

.file-name {
  font-size: 11px;
  margin-top: 6px;
  text-align: center;
  word-break: break-all;
  max-width: 96px;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  color: var(--ink-1);
  font-weight: 500;
}

.file-meta { font-size: 10px; color: var(--ink-3); margin-top: 2px; }
.file-date { font-size: 9px; color: var(--ink-3); margin-top: 1px; }

.rename-input {
  background: var(--bg-2, #1a1a2e);
  border: 1px solid var(--accent);
  border-radius: 4px;
  color: var(--ink-1);
  font-size: 11px;
  padding: 2px 6px;
  outline: none;
  text-align: center;
  max-width: 96px;
  margin-top: 4px;
  font-family: inherit;

  &:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
  }
}

.rename-input-list {
  text-align: left;
  max-width: none;
  width: 100%;
  font-size: 13px;
  margin-top: 0;
}

.list-view { padding: 0; }

.list-header {
  min-height: 32px;
  border-bottom: 1px solid var(--separator);
  background: rgba(255, 255, 255, 0.015);
  cursor: default;
  user-select: none;

  &:hover { background: rgba(255, 255, 255, 0.015) !important; }
}

.list-header-text {
  font-size: 10px;
  color: var(--ink-3);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.list-item {
  min-height: 38px;
  border-radius: 6px;
  margin: 0 4px;
}

.list-item-selected { background: var(--accent-soft) !important; }

.list-name { font-size: 13px; font-weight: 500; color: var(--ink-1); }
.list-meta { font-size: 11px; color: var(--ink-3); font-family: 'Inter', sans-serif; }

.list-thumb {
  width: 24px;
  height: 24px;
  border-radius: 3px;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.cut-item { opacity: 0.45; }

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 60px 0;
  color: var(--ink-3);
  font-size: 13px;
}

.files-status {
  height: 26px;
  display: flex;
  align-items: center;
  padding: 0 14px;
  font-size: 11px;
  color: var(--ink-3);
  border-top: 1px solid var(--separator);
  flex-shrink: 0;
}

.status-path { font-family: 'Inter', sans-serif; font-size: 10px; }
.status-clipboard { color: var(--accent); font-weight: 500; }

// Context menu
.context-menu-overlay {
  position: fixed;
  inset: 0;
  z-index: 9998;
}

.context-menu {
  background: var(--bg-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: var(--shadow-elevated);
  z-index: 9999;

  .q-list {
    padding: 3px 0;
  }

  .q-item {
    min-height: 26px;
    padding: 2px 10px;
    border-radius: 4px;
    margin: 0 4px;
    color: var(--ink-1);

    &:hover {
      background: var(--accent-soft);
    }

    .q-item__section--avatar {
      min-width: 22px !important;
      padding-right: 6px;

      .q-icon { font-size: 14px !important; }
    }

    .q-item__section--side {
      padding-left: 12px;
    }
  }

  .q-separator {
    margin: 3px 8px !important;
  }
}

.ctx-label { font-size: 12px; font-weight: 500; }
.ctx-shortcut { font-size: 10px; color: var(--ink-3); font-family: 'Inter', sans-serif; }
</style>
