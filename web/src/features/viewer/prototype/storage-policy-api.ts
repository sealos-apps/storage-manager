import type { StorageClass, ViewerAPI } from '@/features/viewer/types/viewer'
import { createFakeViewerAPI, storageClassFixture } from '@/features/viewer/test/fakes'

/** Local-only adapter. All changes are in memory; it never calls the cluster. */
export function createStoragePolicyPrototypeAPI(): ViewerAPI {
	let items: StorageClass[] = [
		storageClassFixture({ name: 'csi-rbd-sc', display_names: { zh: '高性能云盘', en: 'Performance disk' }, storage_mode: 'exclusive', available_to_users: true, in_use_pvc_count: 128, provisioner: 'rbd.csi.ceph.com' }),
		storageClassFixture({ name: 'csi-cephfs-sc', display_names: { zh: '共享文件存储', en: 'Shared files' }, storage_mode: 'shared', available_to_users: true, in_use_pvc_count: 36, provisioner: 'cephfs.csi.ceph.com', is_default: false }),
		storageClassFixture({ name: 'nfs-team', display_names: { zh: '团队共享存储', en: 'Team files' }, storage_mode: 'shared', provisioner: 'nfs.csi.k8s.io', is_default: false }),
		storageClassFixture({ name: 'local-path', display_names: { zh: '本地标准存储', en: 'Local disk' }, storage_mode: 'exclusive', in_use_pvc_count: 12, provisioner: 'rancher.io/local-path', is_default: false, allow_volume_expansion: false }),
		storageClassFixture({ name: 'csi-archive', provisioner: 'nfs.csi.k8s.io', is_default: false }),
	]
	function find(name: string) {
		const item = items.find(item => item.name === name)
		if (!item) {
			throw new Error('Storage type not found in prototype')
		}
		return item
	}
	async function unsupported(): Promise<never> {
		throw new Error('此原型仅演示存储类型配置；此操作不执行。')
	}
	return createFakeViewerAPI({
		storageModePrototype: true,
		adminCapabilities: async () => ({ can_manage_pvcs: false, can_manage_storage_classes: true, file_management_enabled: false, pvc_creation_enabled: false, user_namespace: 'ns-admin' }),
		adminListStorageClasses: async () => items.map(item => ({ ...item })),
		listStorageClasses: async () => items.filter(item => item.available_to_users),
		listPVCs: async () => [],
		adminUpdateStorageClassMetadata: async (name, input) => {
			const current = find(name)
			if (!input.storageMode || Object.keys(input.displayNames).length === 0) {
				throw new Error('请补齐展示名称和存储属性。')
			}
			if (current.in_use_pvc_count > 0 && current.storage_mode && current.storage_mode !== input.storageMode) {
				throw new Error('使用中的存储属性暂不允许修改（原型建议）。')
			}
			const next = { ...current, display_names: { ...input.displayNames }, available_to_users: input.availableToUsers, storage_mode: input.storageMode }
			items = items.map(item => item.name === name ? next : item)
			return next
		},
		adminDescribeStorageClass: async name => ({ name, describe: `Name: ${name}\nProvisioner: ${find(name).provisioner}\nPVC usage: ${find(name).in_use_pvc_count}\n\nLocal prototype data only.` }),
		adminGetStorageClassYAML: async name => ({ name, yaml: `apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: ${name}\nprovisioner: ${find(name).provisioner}\nreclaimPolicy: ${find(name).reclaim_policy}\n` }),
		adminCreateStorageClass: unsupported,
		adminUpdateStorageClass: unsupported,
		adminDeleteStorageClass: unsupported,
		createPVC: unsupported,
		deletePVC: unsupported,
		expandPVC: unsupported,
		updatePVC: unsupported,
		createViewerSession: unsupported,
	})
}
