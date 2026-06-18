import type { ReactNode } from 'react'

import * as monaco from 'monaco-editor'
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import cssWorker from 'monaco-editor/esm/vs/language/css/css.worker?worker'
import htmlWorker from 'monaco-editor/esm/vs/language/html/html.worker?worker'
import jsonWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker'
import tsWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker'
import { createElement, useEffect, useRef } from 'react'

const CSSWorker = cssWorker
const EditorWorker = editorWorker
const HTMLWorker = htmlWorker
const JSONWorker = jsonWorker
const TSWorker = tsWorker

interface MonacoEditorProps {
	height?: number | string
	language?: string
	loading?: ReactNode
	onChange?: (value: string | undefined, ev: monaco.editor.IModelContentChangedEvent) => void
	options?: monaco.editor.IStandaloneEditorConstructionOptions
	value?: string
	width?: number | string
}

globalThis.MonacoEnvironment = {
	getWorker(_workerID, label) {
		if (label === 'json') {
			return new JSONWorker()
		}
		if (label === 'css' || label === 'scss' || label === 'less') {
			return new CSSWorker()
		}
		if (label === 'html' || label === 'handlebars' || label === 'razor') {
			return new HTMLWorker()
		}
		if (label === 'typescript' || label === 'javascript') {
			return new TSWorker()
		}
		return new EditorWorker()
	},
}

export default function MonacoEditor({
	height = '100%',
	language,
	onChange,
	options,
	value = '',
	width = '100%',
}: MonacoEditorProps) {
	const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
	const hostRef = useRef<HTMLDivElement | null>(null)
	const languageRef = useRef(language)
	const onChangeRef = useRef(onChange)
	const optionsRef = useRef(options)
	const valueRef = useRef(value)

	languageRef.current = language
	onChangeRef.current = onChange
	optionsRef.current = options
	valueRef.current = value

	useEffect(() => {
		if (!hostRef.current) {
			return undefined
		}

		const editor = monaco.editor.create(hostRef.current, {
			...optionsRef.current,
			automaticLayout: true,
			language: languageRef.current,
			value: valueRef.current,
		})
		editorRef.current = editor
		const changeSubscription = editor.onDidChangeModelContent((event) => {
			onChangeRef.current?.(editor.getValue(), event)
		})

		return () => {
			const model = editor.getModel()
			changeSubscription.dispose()
			editor.dispose()
			model?.dispose()
			editorRef.current = null
		}
	}, [])

	useEffect(() => {
		const editor = editorRef.current
		if (!editor) {
			return
		}
		editor.updateOptions(options ?? {})
	}, [options])

	useEffect(() => {
		const model = editorRef.current?.getModel()
		if (!model || !language) {
			return
		}
		monaco.editor.setModelLanguage(model, language)
	}, [language])

	useEffect(() => {
		const editor = editorRef.current
		if (!editor || editor.getValue() === value) {
			return
		}
		editor.setValue(value)
	}, [value])

	return createElement('div', { ref: hostRef, style: { height, width } })
}
