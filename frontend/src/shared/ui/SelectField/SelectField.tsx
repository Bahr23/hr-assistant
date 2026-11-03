import { Check, ChevronDown } from 'lucide-react'
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'

import { cn } from '@/shared/lib/cn'
import styles from './SelectField.module.css'

const MENU_MAX_HEIGHT = 240
const MENU_MIN_HEIGHT = 80
const MENU_EDGE_GAP = 8

function clippingBoundary(element: HTMLElement) {
  let parent = element.parentElement

  while (parent) {
    const { overflowY } = window.getComputedStyle(parent)
    if (/(auto|scroll|hidden)/.test(overflowY)) return parent.getBoundingClientRect()
    parent = parent.parentElement
  }

  return { top: 0, bottom: window.innerHeight }
}

export type SelectOption<T extends string = string> = {
  disabled?: boolean
  label: string
  value: T
}

type SelectFieldProps<T extends string> = {
  className?: string
  disabled?: boolean
  error?: string
  hint?: string
  id?: string
  label: string
  name?: string
  onValueChange: (value: T) => void
  options: readonly SelectOption<T>[]
  required?: boolean
  value: T
}

export function SelectField<T extends string>({
  className,
  disabled = false,
  error,
  hint,
  id,
  label,
  name,
  onValueChange,
  options,
  required = false,
  value,
}: SelectFieldProps<T>) {
  const generatedId = useId()
  const fieldId = id ?? name ?? generatedId
  const labelId = `${fieldId}-label`
  const listboxId = `${fieldId}-listbox`
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined
  const rootRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const [menuPlacement, setMenuPlacement] = useState<'above' | 'below'>('below')
  const [menuMaxHeight, setMenuMaxHeight] = useState(MENU_MAX_HEIGHT)
  const selectedIndex = useMemo(() => options.findIndex((option) => option.value === value), [options, value])
  const [activeIndex, setActiveIndex] = useState(Math.max(selectedIndex, 0))
  const selectedOption = selectedIndex >= 0 ? options[selectedIndex] : undefined

  useEffect(() => {
    if (!open) return

    const closeOnOutsideClick = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }

    document.addEventListener('pointerdown', closeOnOutsideClick)
    return () => document.removeEventListener('pointerdown', closeOnOutsideClick)
  }, [open])

  useEffect(() => {
    if (!open) return

    const updateMenuPosition = () => {
      const root = rootRef.current
      if (!root) return

      const rootRect = root.getBoundingClientRect()
      const boundary = clippingBoundary(root)
      const boundaryTop = Math.max(0, boundary.top)
      const boundaryBottom = Math.min(window.innerHeight, boundary.bottom)
      const spaceAbove = Math.max(0, rootRect.top - boundaryTop - MENU_EDGE_GAP)
      const spaceBelow = Math.max(0, boundaryBottom - rootRect.bottom - MENU_EDGE_GAP)
      const expectedHeight = Math.min(MENU_MAX_HEIGHT, options.length * 44 + 16)
      const placement = spaceBelow < expectedHeight && spaceAbove > spaceBelow ? 'above' : 'below'
      const availableSpace = placement === 'above' ? spaceAbove : spaceBelow

      setMenuPlacement(placement)
      setMenuMaxHeight(Math.max(MENU_MIN_HEIGHT, Math.min(MENU_MAX_HEIGHT, availableSpace)))
    }

    updateMenuPosition()
    window.addEventListener('resize', updateMenuPosition)
    window.addEventListener('scroll', updateMenuPosition, true)
    return () => {
      window.removeEventListener('resize', updateMenuPosition)
      window.removeEventListener('scroll', updateMenuPosition, true)
    }
  }, [open, options.length])

  const firstEnabledIndex = () => options.findIndex((option) => !option.disabled)
  const lastEnabledIndex = () => {
    for (let index = options.length - 1; index >= 0; index -= 1) {
      if (!options[index]?.disabled) return index
    }
    return -1
  }
  const nextEnabledIndex = (start: number, direction: 1 | -1) => {
    if (options.length === 0) return -1
    for (let offset = 1; offset <= options.length; offset += 1) {
      const index = (start + direction * offset + options.length) % options.length
      if (!options[index]?.disabled) return index
    }
    return -1
  }
  const selectOption = (index: number) => {
    const option = options[index]
    if (!option || option.disabled) return
    onValueChange(option.value)
    setActiveIndex(index)
    setOpen(false)
    buttonRef.current?.focus()
  }
  const openMenu = () => {
    if (disabled) return
    setActiveIndex(selectedIndex >= 0 ? selectedIndex : Math.max(firstEnabledIndex(), 0))
    setOpen(true)
  }
  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'Escape') {
      setOpen(false)
      return
    }
    if (event.key === 'Tab') {
      setOpen(false)
      return
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      if (open) selectOption(activeIndex)
      else openMenu()
      return
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!open) {
        openMenu()
        return
      }
      const nextIndex = nextEnabledIndex(activeIndex, event.key === 'ArrowDown' ? 1 : -1)
      if (nextIndex >= 0) setActiveIndex(nextIndex)
      return
    }
    if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      if (!open) openMenu()
      const nextIndex = event.key === 'Home' ? firstEnabledIndex() : lastEnabledIndex()
      if (nextIndex >= 0) setActiveIndex(nextIndex)
    }
  }

  return (
    <div ref={rootRef} className={cn(styles.field, className)}>
      <span className={styles.label} id={labelId}>{label}</span>
      {name ? <input type="hidden" name={name} value={value} /> : null}
      <button
        ref={buttonRef}
        id={fieldId}
        type="button"
        className={cn(styles.trigger, open && styles.open, error && styles.invalid)}
        role="combobox"
        aria-controls={listboxId}
        aria-describedby={describedBy}
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-invalid={Boolean(error)}
        aria-labelledby={labelId}
        aria-required={required}
        disabled={disabled}
        onClick={() => open ? setOpen(false) : openMenu()}
        onKeyDown={handleKeyDown}
      >
        <span>{selectedOption?.label ?? 'Выберите значение'}</span>
        <ChevronDown className={styles.chevron} aria-hidden="true" />
      </button>
      {open ? (
        <div
          className={cn(styles.menu, menuPlacement === 'above' && styles.menuAbove)}
          id={listboxId}
          role="listbox"
          aria-labelledby={labelId}
          data-placement={menuPlacement}
          style={{ maxHeight: menuMaxHeight }}
        >
          {options.map((option, index) => (
            <button
              key={option.value}
              type="button"
              className={cn(styles.option, index === activeIndex && styles.activeOption)}
              role="option"
              aria-selected={option.value === value}
              disabled={option.disabled}
              onMouseEnter={() => { if (!option.disabled) setActiveIndex(index) }}
              onClick={() => selectOption(index)}
            >
              <span>{option.label}</span>
              {option.value === value ? <Check aria-hidden="true" /> : null}
            </button>
          ))}
        </div>
      ) : null}
      {error ? (
        <span className={styles.error} id={`${fieldId}-error`} role="alert">{error}</span>
      ) : hint ? (
        <span className={styles.hint} id={`${fieldId}-hint`}>{hint}</span>
      ) : null}
    </div>
  )
}
