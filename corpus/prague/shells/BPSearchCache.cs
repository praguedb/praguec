namespace Common.IntegratedEventApi.Cache.Prague.Models;

public sealed partial class BPSearchCache
{
    /// <summary>Remove all entries by iterating current values.</summary>
    public void Clear()
    {
        foreach (var item in Query().Execute())
        {
            Remove(item.Key);
        }
    }
}