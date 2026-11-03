import { moveItem } from './moveItem'

describe('moveItem', () => {
  it('moves an item forward without mutating the source', () => {
    const source = ['one', 'two', 'three']

    expect(moveItem(source, 0, 2)).toEqual(['two', 'three', 'one'])
    expect(source).toEqual(['one', 'two', 'three'])
  })

  it('moves an item backward', () => {
    expect(moveItem(['one', 'two', 'three'], 2, 0)).toEqual(['three', 'one', 'two'])
  })
})
