namespace Common.EventApi.Cache.Prague.Models;

public sealed partial class BPShowCache
{
    /// <summary>Remove all entries by iterating current values.</summary>
    public void Clear()
    {
        foreach (var item in Query().Execute())
        {
            Remove(item.EventId);
        }
    }
}