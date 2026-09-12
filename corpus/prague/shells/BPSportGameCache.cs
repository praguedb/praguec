namespace Common.IntegratedEventApi.Cache.Prague.Models;

public sealed partial class BPSportGameCache
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